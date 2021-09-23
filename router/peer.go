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
	allocated    atomic.Bool               //
	started      atomic.Bool               // worker goroutines started?
	wg           *sync.WaitGroup           // wait group for worker goroutines
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
	announcement *rootAnnouncementWithTime // last received announcement from peer
	statistics   peerStatistics            //
}

func (p *Peer) start() {
	if !p.started.CAS(false, true) {
		return
	}

	// Store the fact that we're connected to this public key in
	// this zone, so that the multicast code can ignore nodes we
	// are already connected to.
	index := hex.EncodeToString(p.public[:]) + p.zone
	p.r.active.Store(index, p.port)

	// When the peer dies, we need to clean up.
	var lasterr error
	var lasterrMutex sync.Mutex
	defer func() {
		p.r.callbacks.onDisconnected(p.port, p.public, p.peertype, lasterr)
		p.reset()
		p.r.active.Delete(index)
	}()

	// Push a root update to our new peer. This will notify them
	// of our coordinates and that we are alive.
	p.protoOut.push(p.r.tree.Root().ForPeer(p))

	// Start the reader and writer goroutines for this peer.
	p.wg.Add(2)
	go func() {
		if err := p.reader(p.context); err != nil && err != context.Canceled {
			lasterrMutex.Lock()
			lasterr = fmt.Errorf("reader error: %w", err)
			lasterrMutex.Unlock()
		}
	}()
	go func() {
		if err := p.writer(p.context); err != nil && err != context.Canceled {
			lasterrMutex.Lock()
			lasterr = fmt.Errorf("writer error: %w", err)
			lasterrMutex.Unlock()
		}
	}()

	// Report the new connection.
	if p.port != 0 {
		p.r.dht.insertNode(p)
	}
	if p.r.simulator != nil {
		p.r.simulator.ReportNewLink(p.conn, p.r.public, p.public)
	}
	p.r.callbacks.onConnected(p.port, p.public, p.peertype)

	p.r.log.Printf("Connected port %d to %s (zone %q)\n", p.port, p.conn.RemoteAddr(), p.zone)

	// Wait for the cancellation, and then for the goroutines to stop.
	<-p.context.Done()
	p.started.Store(false)

	// Notify the tree and DHT about the peer cancellation. We need
	// to do this quickly so that they have more time to recover, even
	// if the goroutines take a while to stop.
	p.r.snake.portWasDisconnected(p.port)
	p.r.tree.portWasDisconnected(p.port)

	// Now wait for the goroutines to stop.
	p.wg.Wait()

	// Report the disconnection.
	if p.port != 0 {
		p.r.dht.deleteNode(p.public)
	}
	if p.r.simulator != nil {
		p.r.simulator.ReportDeadLink(p.r.public, p.public)
	}

	// Make sure the connection is closed.
	_ = p.conn.Close()

	// ... and finally, yell about it.
	lasterrMutex.Lock()
	if lasterr != nil {
		p.r.log.Printf("Disconnected port %d: %s\n", p.port, lasterr)
	} else {
		p.r.log.Printf("Disconnected port %d\n", p.port)
	}
	lasterrMutex.Unlock()
}

func (p *Peer) reset() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.allocated.Store(false)
	p.started.Store(false)
	p.zone = ""
	p.peertype = 0
	p.context, p.cancel = nil, nil
	p.conn = nil
	p.public = types.PublicKey{}
	p.trafficOut.reset()
	p.protoOut.reset()
	p.statistics.reset()
	p.coords = nil
	p.announcement = nil
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
	return p.announcement != nil && time.Since(p.announcement.receiveTime) < announcementTimeout
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
	if p.announcement != nil {
		if new.RootPublicKey == p.announcement.RootPublicKey && new.Sequence < p.announcement.Sequence {
			p.announcement = nil
			p.coords = nil
			return fmt.Errorf("root announcement replays sequence number")
		}
	}
	p.r.tree.ordering++
	p.announcement = &rootAnnouncementWithTime{
		SwitchAnnouncement: *new,
		receiveTime:        time.Now(),
		receiveOrder:       p.r.tree.ordering,
	}
	p.coords = new.PeerCoords()
	return nil
}

func (p *Peer) lastAnnouncement() *rootAnnouncementWithTime {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	switch {
	case !p.allocated.Load():
		return nil
	case !p.started.Load():
		return nil
	case p.announcement != nil && time.Since(p.announcement.receiveTime) >= announcementTimeout:
		return nil
	}
	return p.announcement
}

func (p *Peer) stop() {
	if p.started.CAS(true, false) {
		p.mutex.Lock()
		p.cancel()
		p.mutex.Unlock()
	}
}

func (p *Peer) generateKeepalive() *types.Frame {
	frame := types.GetFrame()
	frame.Version = types.Version0
	frame.Type = types.TypeKeepalive
	return frame
}

func (p *Peer) reader(ctx context.Context) error {
	defer p.wg.Done()
	defer p.cancel()

	buf := make([]byte, types.MaxFrameSize)
	for {
		select {
		case <-ctx.Done():
			// The switch peer is shutting down.
			return context.Canceled

		default:
			if p.port != 0 {
				if err := p.conn.SetReadDeadline(time.Now().Add(PeerKeepaliveTimeout)); err != nil {
					return fmt.Errorf("conn.SetReadDeadline: %w", err)
				}
			}
			if _, err := io.ReadFull(p.conn, buf[:8]); err != nil {
				return fmt.Errorf("p.conn.Peek: %w", err)
			}
			if err := p.conn.SetReadDeadline(time.Time{}); err != nil {
				return fmt.Errorf("conn.SetReadDeadline: %w", err)
			}
			if !bytes.Equal(buf[:4], types.FrameMagicBytes) {
				return fmt.Errorf("missing magic bytes")
			}
			expecting := int(binary.BigEndian.Uint16(buf[6:8]))
			n, err := io.ReadFull(p.conn, buf[8:expecting])
			if err != nil {
				return fmt.Errorf("io.ReadFull: %w", err)
			}
			if n < expecting-8 {
				p.r.log.Println("Expecting", expecting, "bytes but got", n, "bytes")
				continue
			}
			func(frame *types.Frame) {
				defer frame.Done()
				if _, err := frame.UnmarshalBinary(buf[:n+8]); err != nil {
					p.r.log.Println("frame.UnmarshalBinary:", err)
					return
				}
				switch {
				case frame.Version != types.Version0:
					fallthrough
				case frame.Type == types.TypeKeepalive:
					return
				}
				for _, port := range p.getNextHops(frame, p.port) {
					dest := p.r.ports[port]
					if !dest.started.Load() || (dest.port != 0 && !dest.Alive()) {
						// Ignore ports that are not good candidates.
						continue
					}
					switch frame.Type {
					case types.TypePathfind, types.TypeVirtualSnakePathfind:
						signedframe, err := p.r.signPathfind(frame.Borrow(), p, dest)
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
			}(types.GetFrame())
		}
	}
}

var bufPool = sync.Pool{
	New: func() interface{} {
		b := [types.MaxFrameSize]byte{}
		return &b
	},
}

func (p *Peer) writer(ctx context.Context) error {
	defer p.wg.Done()
	defer p.cancel()

	tick := time.NewTicker(PeerKeepaliveInterval)
	defer tick.Stop()

	send := func(frame *types.Frame) error {
		if frame == nil {
			return nil
		}
		buf := bufPool.Get().(*[types.MaxFrameSize]byte)
		defer bufPool.Put(buf) // nolint:staticcheck
		fn, err := frame.MarshalBinary(buf[:])
		frame.Done()
		if err != nil {
			return nil
		}
		if !bytes.Equal(buf[:4], types.FrameMagicBytes) {
			return nil
		}
		if err := p.conn.SetWriteDeadline(time.Now().Add(PeerKeepaliveTimeout)); err != nil {
			return fmt.Errorf("p.conn.SetWriteDeadline: %w", err)
		}
		if _, err = p.conn.Write(buf[:fn]); err != nil {
			return fmt.Errorf("p.conn.Write: %w", err)
		}
		if err := p.conn.SetWriteDeadline(time.Time{}); err != nil {
			return fmt.Errorf("p.conn.SetWriteDeadline: %w", err)
		}
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case frame := <-p.protoOut.pop():
			if err := send(frame); err != nil {
				return fmt.Errorf("send: %w", err)
			}
			p.protoOut.ack()
			p.statistics.txProtoSuccessful.Inc()
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		case frame := <-p.protoOut.pop():
			if err := send(frame); err != nil {
				return fmt.Errorf("send: %w", err)
			}
			p.protoOut.ack()
			p.statistics.txProtoSuccessful.Inc()
			continue
		case <-p.trafficOut.wait():
			if frame, ok := p.trafficOut.pop(); ok && send(frame) == nil {
				p.statistics.txTrafficSuccessful.Inc()
			} else {
				p.statistics.txTrafficDropped.Inc()
			}
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		case frame := <-p.protoOut.pop():
			if err := send(frame); err != nil {
				return fmt.Errorf("send: %w", err)
			}
			p.protoOut.ack()
			p.statistics.txProtoSuccessful.Inc()
			continue
		case <-p.trafficOut.wait():
			if frame, ok := p.trafficOut.pop(); ok && send(frame) == nil {
				p.statistics.txTrafficSuccessful.Inc()
			} else {
				p.statistics.txTrafficDropped.Inc()
			}
			continue
		case <-tick.C:
			if err := send(p.generateKeepalive()); err != nil {
				return fmt.Errorf("send: %w", err)
			}
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

// The mutexes should be held on the ports before sorting. It
// is more efficient to hold them already for each port rather
// than taking and releasing the mutexes for each sort operation.
func (p peers) Less(i, j int) bool {
	if p[i].peertype < p[j].peertype {
		return true
	}
	return p[i].port < p[j].port
}

var peersPool = &sync.Pool{
	New: func() interface{} {
		return make(peers, 0, PortCount)
	},
}
