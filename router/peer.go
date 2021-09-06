// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package router

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

const PeerKeepaliveInterval = time.Second * 3
const PeerKeepaliveTimeout = PeerKeepaliveInterval * 3

const (
	PeerTypeMulticast int = iota
	PeerTypeBluetooth
	PeerTypeRemote
)

type Peer struct {
	r            *Router                   //
	port         types.SwitchPortID        //
	started      atomic.Bool               // worker goroutines started?
	child        atomic.Bool               // is this node a child of ours?
	mutex        sync.RWMutex              // protects everything below this line
	zone         string                    //
	peertype     int                       //
	context      context.Context           //
	cancel       context.CancelFunc        //
	conn         net.Conn                  // underlying connection to peer
	public       types.PublicKey           //
	trafficOut   *lifoQueue                // queue traffic message to peer
	protoOut     *fifoQueue                // queue protocol message to peer
	coords       types.SwitchPorts         //
	announcement *rootAnnouncementWithTime //
	statistics   peerStatistics            //
}

func (p *Peer) reset() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.started.Store(false)
	p.child.Store(false)
	p.zone = ""
	p.peertype = 0
	p.context, p.cancel = nil, nil
	//p.conn = nil
	p.public = types.PublicKey{}
	p.trafficOut.reset()
	p.protoOut.reset()
	p.coords = nil
	p.announcement = nil
	p.statistics.reset()
}

type peerStatistics struct {
	txProtoSuccessful   atomic.Uint64
	txProtoDropped      atomic.Uint64
	txTrafficSuccessful atomic.Uint64
	txTrafficDropped    atomic.Uint64
	rxProto             atomic.Uint64
	rxTraffic           atomic.Uint64
}

func (s *peerStatistics) reset() {
	s.txProtoSuccessful.Store(0)
	s.txProtoDropped.Store(0)
	s.txTrafficSuccessful.Store(0)
	s.txTrafficDropped.Store(0)
	s.rxProto.Store(0)
	s.rxTraffic.Store(0)
}

func (p *Peer) IsParent() bool {
	return p.r.tree.Parent() == p.port
}

func (p *Peer) Alive() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.announcement != nil && time.Since(p.announcement.at) < announcementTimeout
}

func (p *Peer) PublicKey() types.PublicKey {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.public
}

func (p *Peer) Coordinates() types.SwitchPorts {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.coords
}

func (p *Peer) SeenCommonRootRecently() bool {
	if !p.Alive() {
		return false
	}
	last := p.lastAnnouncement()
	if last == nil {
		return false
	}
	lpk := last.RootPublicKey
	rpk := p.r.RootPublicKey()
	return lpk == rpk
}

func (p *Peer) updateAnnouncement(new *types.SwitchAnnouncement) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	coords, err := new.PeerCoords(p.public)
	if err != nil {
		p.announcement = nil
		p.coords = nil
		return fmt.Errorf("new.PeerCoords: %w", err)
	}
	p.announcement = &rootAnnouncementWithTime{
		SwitchAnnouncement: *new,
		at:                 time.Now(),
	}
	p.coords = coords
	isChild := false
	for _, sig := range new.Signatures {
		if sig.PublicKey.EqualTo(p.r.public) {
			isChild = true
			break
		}
	}
	p.child.Store(isChild)
	return nil
}

func (p *Peer) lastAnnouncement() *rootAnnouncementWithTime {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	switch {
	case !p.started.Load():
		return nil
	case p.announcement != nil && time.Since(p.announcement.at) >= announcementTimeout:
		return nil
	}
	return p.announcement
}

func (p *Peer) start() error {
	if !p.started.CAS(false, true) {
		return errors.New("switch peer is already started")
	}
	p.r.active.Store(hex.EncodeToString(p.public[:])+p.zone, p.port)
	go p.reader(p.context)
	go p.writer(p.context)
	return nil
}

func (p *Peer) stop() error {
	if !p.started.CAS(true, false) {
		return errors.New("switch peer is already stopped")
	}
	defer p.r.active.Delete(hex.EncodeToString(p.public[:]) + p.zone)
	p.mutex.Lock()
	_ = p.conn.Close()
	p.cancel()
	p.mutex.Unlock()
	p.reset()
	return nil
}

func (p *Peer) generateKeepalive() *types.Frame {
	frame := types.GetFrame()
	frame.Version = types.Version0
	frame.Type = types.TypeKeepalive
	return frame
}

func (p *Peer) generateAnnouncement() *types.Frame {
	if p.port == 0 {
		return nil
	}
	announcement := p.r.tree.Root()
	for _, sig := range announcement.Signatures {
		if p.r.public.EqualTo(sig.PublicKey) {
			// For some reason the announcement that we want to send already
			// includes our signature. This shouldn't really happen but if we
			// did send it, other nodes would end up ignoring the announcement
			// anyway since it would appear to be a routing loop.
			return nil
		}
	}
	// Sign the announcement.
	if err := announcement.Sign(p.r.private[:], p.port); err != nil {
		p.r.log.Println("Failed to sign switch announcement:", err)
		return nil
	}
	var payload [MaxPayloadSize]byte
	n, err := announcement.MarshalBinary(payload[:])
	if err != nil {
		p.r.log.Println("Failed to marshal switch announcement:", err)
		return nil
	}
	frame := types.GetFrame()
	frame.Version = types.Version0
	frame.Type = types.TypeSTP
	frame.Destination = types.SwitchPorts{}
	frame.Payload = payload[:n]
	return frame
}

func (p *Peer) reader(ctx context.Context) {
	buf := make([]byte, MaxFrameSize)
	for {
		select {
		case <-ctx.Done():
			// The switch peer is shutting down.
			return

		default:
			if p.port != 0 {
				if err := p.conn.SetReadDeadline(time.Now().Add(PeerKeepaliveTimeout)); err != nil {
					_ = p.r.Disconnect(p.port, fmt.Errorf("conn.SetReadDeadline: %w", err))
					return
				}
			}
			if _, err := io.ReadFull(p.conn, buf[:8]); err != nil {
				_ = p.r.Disconnect(p.port, fmt.Errorf("p.conn.Peek: %w", err))
				return
			}
			if !bytes.Equal(buf[:4], types.FrameMagicBytes) {
				_ = p.r.Disconnect(p.port, fmt.Errorf("missing magic bytes"))
				return
			}
			expecting := int(binary.BigEndian.Uint16(buf[6:8]))
			n, err := io.ReadFull(p.conn, buf[8:expecting])
			if err != nil {
				_ = p.r.Disconnect(p.port, fmt.Errorf("io.ReadFull: %w", err))
				return
			}
			if n < expecting-8 {
				p.r.log.Println("Expecting", expecting, "bytes but got", n, "bytes")
				continue
			}
			frame := types.GetFrame()
			if _, err := frame.UnmarshalBinary(buf[:n+8]); err != nil {
				p.r.log.Println("Port", p.port, "error unmarshalling frame:", err)
				frame.Done()
				return
			}
			if frame.Version != types.Version0 {
				p.r.log.Println("Port", p.port, "incorrect version in frame")
				frame.Done()
				return
			}
			if frame.Type == types.TypeKeepalive {
				frame.Done()
				continue
			}
			func(frame *types.Frame) {
				defer frame.Done()
				//	p.r.log.Println("Frame type", frame.Type.String(), frame.DestinationKey)
				for _, port := range p.getNextHops(frame, p.port) {
					// Ignore ports that are not good candidates.
					dest := p.r.ports[port]
					if !dest.started.Load() || (dest.port != 0 && !dest.Alive()) {
						continue
					}
					if p.port != 0 && dest.port != 0 {
						if p.port == dest.port || p.public.EqualTo(dest.public) {
							continue
						}
					}
					switch frame.Type {
					case types.TypePathfind, types.TypeVirtualSnakePathfind:
						signedframe, err := p.r.signPathfind(frame, p, dest)
						if err != nil {
							p.r.log.Println("WARNING: Failed to sign pathfind:", err)
							continue
						}
						if signedframe == nil {
							continue
						}
						if dest.trafficOut.push(signedframe) {
							return
						} else {
							signedframe.Done()
							continue
						}

					case types.TypeDHTRequest, types.TypeDHTResponse, types.TypeVirtualSnakeBootstrap, types.TypeVirtualSnakeBootstrapACK, types.TypeVirtualSnakeSetup, types.TypeVirtualSnakeTeardown:
						if dest.protoOut.push(frame.Borrow()) {
							return
						} else {
							frame.Done()
							continue
						}

					case types.TypeGreedy, types.TypeSource, types.TypeVirtualSnake:
						if dest.trafficOut.push(frame.Borrow()) {
							return
						} else {
							frame.Done()
							continue
						}
					}
				}
			}(frame)
		}
	}
}

var bufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, MaxFrameSize)
	},
}

func (p *Peer) writer(ctx context.Context) {
	tick := time.NewTicker(PeerKeepaliveInterval)
	defer tick.Stop()
	send := func(frame *types.Frame) {
		if frame == nil {
			return
		}
		buf := bufPool.Get().([]byte)
		defer bufPool.Put(buf) // nolint:staticcheck
		fn, err := frame.MarshalBinary(buf)
		frame.Done()
		if err != nil {
			p.r.log.Println("Port", p.port, "error marshalling frame:", err)
			return
		}
		if !bytes.Equal(buf[:4], types.FrameMagicBytes) {
			panic("expected magic bytes")
		}
		if _, err = p.conn.Write(buf[:fn]); err != nil {
			_ = p.r.Disconnect(p.port, fmt.Errorf("p.conn.Write: %w", err))
			return
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-p.protoOut.pop():
			p.protoOut.ack()
			send(frame)
			p.statistics.txProtoSuccessful.Inc()
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return
		case frame := <-p.protoOut.pop():
			p.protoOut.ack()
			send(frame)
			p.statistics.txProtoSuccessful.Inc()
			continue
		case <-p.trafficOut.wait():
			if frame, ok := p.trafficOut.pop(); ok {
				send(frame)
				p.statistics.txTrafficSuccessful.Inc()
			} else {
				p.statistics.txTrafficDropped.Inc()
			}
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return
		case frame := <-p.protoOut.pop():
			p.protoOut.ack()
			send(frame)
			p.statistics.txProtoSuccessful.Inc()
			continue
		case <-p.trafficOut.wait():
			if frame, ok := p.trafficOut.pop(); ok {
				send(frame)
				p.statistics.txTrafficSuccessful.Inc()
			} else {
				p.statistics.txTrafficDropped.Inc()
			}
			continue
		case <-tick.C:
			send(p.generateKeepalive())
			p.statistics.txProtoSuccessful.Inc()
			continue
		}
	}
}

type peers []*Peer

func (p peers) Len() int {
	return len(p)
}

func (p peers) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p peers) Less(i, j int) bool {
	p[i].mutex.RLock()
	p[j].mutex.RLock()
	defer p[i].mutex.RUnlock()
	defer p[j].mutex.RUnlock()
	if p[i].peertype < p[j].peertype {
		return true
	}
	return p[i].port < p[j].port
}
