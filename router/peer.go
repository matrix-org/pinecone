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
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

const PeerKeepaliveInterval = time.Second * 2
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
	alive        atomic.Bool               // have we received a handshake?
	mutex        sync.RWMutex              // protects everything below this line
	zone         string                    //
	peertype     int                       //
	context      context.Context           //
	cancel       context.CancelFunc        //
	conn         net.Conn                  // underlying connection to peer
	public       types.PublicKey           //
	trafficOut   queue                     // queue traffic message to peer
	protoOut     queue                     // queue protocol message to peer
	coords       types.SwitchPorts         //
	announce     chan struct{}             //
	announcement *rootAnnouncementWithTime //
	statistics   peerStatistics            //
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
	if !p.alive.Load() {
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

func (p *Peer) SeenRecently() bool {
	if last := p.lastAnnouncement(); last != nil {
		return true
	}
	return false
}

func (p *Peer) updateAnnouncement(new *types.SwitchAnnouncement) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	coords, err := new.PeerCoords(p.public)
	if err != nil {
		p.alive.Store(false)
		p.announcement = nil
		p.coords = nil
		return fmt.Errorf("new.PeerCoords: %w", err)
	}
	if p.alive.CAS(false, true) {
		p.r.snake.portWasConnected(p.port)
	}
	p.announcement = &rootAnnouncementWithTime{
		SwitchAnnouncement: *new,
		at:                 time.Now(),
	}
	p.coords = coords
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
	p.alive.Store(false)
	go p.reader(p.context)
	go p.writer(p.context)
	return nil
}

func (p *Peer) stop() error {
	if !p.started.CAS(true, false) {
		return errors.New("switch peer is already stopped")
	}
	p.alive.Store(false)
	p.cancel()
	_ = p.conn.Close()
	return nil
}

/*
func (p *Peer) generateKeepalive() *types.Frame {
	frame := types.GetFrame()
	frame.Version = types.Version0
	frame.Type = types.TypeKeepalive
	return frame
}
*/

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
					if !dest.started.Load() || (dest.port != 0 && !dest.alive.Load()) {
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
	//tick := time.NewTicker(PeerKeepaliveInterval)
	//defer tick.Stop()
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
		remaining := buf[:fn]
		for len(remaining) > 0 {
			n, err := p.conn.Write(remaining)
			if err != nil {
				_ = p.r.Disconnect(p.port, fmt.Errorf("p.conn.Write: %w", err))
				return
			}
			remaining = remaining[n:]
		}
	}

	// The very first thing we send should be a tree announcement,
	// so that the remote side can work out our coords and consider
	// us to be "alive".
	send(p.generateAnnouncement())

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.announce:
			send(p.generateAnnouncement())
			p.statistics.txProtoSuccessful.Inc()
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-p.announce:
			send(p.generateAnnouncement())
			p.statistics.txProtoSuccessful.Inc()
			continue
		case <-p.protoOut.wait():
			if frame, ok := p.protoOut.pop(); ok {
				send(frame)
				p.statistics.txProtoSuccessful.Inc()
			} else {
				p.statistics.txProtoDropped.Inc()
			}
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-p.announce:
			send(p.generateAnnouncement())
			p.statistics.txProtoSuccessful.Inc()
			continue
		case <-p.protoOut.wait():
			if frame, ok := p.protoOut.pop(); ok {
				send(frame)
				p.statistics.txProtoSuccessful.Inc()
			} else {
				p.statistics.txProtoDropped.Inc()
			}
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
		case <-p.announce:
			send(p.generateAnnouncement())
			p.statistics.txProtoSuccessful.Inc()
			continue
		case <-p.protoOut.wait():
			if frame, ok := p.protoOut.pop(); ok {
				send(frame)
				p.statistics.txProtoSuccessful.Inc()
			} else {
				p.statistics.txProtoDropped.Inc()
			}
			continue
		case <-p.trafficOut.wait():
			if frame, ok := p.trafficOut.pop(); ok {
				send(frame)
				p.statistics.txTrafficSuccessful.Inc()
			} else {
				p.statistics.txTrafficDropped.Inc()
			}
			continue
			//case <-tick.C:
			//	send(p.generateKeepalive())
			//  p.statistics.txProtoSuccessful.Inc()
			//	continue
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
