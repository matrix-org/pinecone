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

	"github.com/Arceliar/phony"
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

type peer struct {
	reader   phony.Inbox
	writer   phony.Inbox
	router   *Router            // populated by router
	port     types.SwitchPortID // populated by router
	context  context.Context    // populated by router
	cancel   context.CancelFunc // populated by router
	conn     net.Conn           // populated by router
	zone     string             // populated by router
	peertype int                // populated by router
	public   types.PublicKey    // populated by router
	started  atomic.Bool        // prevents more than one shutdown
	proto    *fifoQueue         // thread-safe
	traffic  *lifoQueue         // thread-safe
}

func (p *peer) send(f *types.Frame) bool {
	if p == nil {
		return false
	}
	switch f.Type {
	// Protocol messages
	case types.TypeSTP, types.TypeKeepalive:
		fallthrough
	case types.TypeVirtualSnakeBootstrap, types.TypeVirtualSnakeBootstrapACK:
		fallthrough
	case types.TypeVirtualSnakeSetup, types.TypeVirtualSnakeTeardown:
		// The local peer doesn't have a protocol queue so we should check
		// for nils to prevent panics.
		if p.proto != nil {
			return p.proto.push(f)
		}

	// Traffic messages
	case types.TypeVirtualSnake, types.TypeGreedy, types.TypeSource:
		fallthrough
	case types.TypeSNEKPing, types.TypeSNEKPong, types.TypeTreePing, types.TypeTreePong:
		return p.traffic.push(f)
	}

	return false
}

var counter = map[types.FrameType]uint64{}
var counterMutex sync.Mutex

func (p *peer) _receive(f *types.Frame) error {
	nexthops := p.router.state.nextHopsFor(p, f)
	deadend := len(nexthops) == 0 || (len(nexthops) == 1 && nexthops[0] == p.router.local)
	defer func() {
		for _, peer := range nexthops {
			if peer.send(f) {
				return
			}
		}
	}()

	counterMutex.Lock()
	counter[f.Type]++
	counterMutex.Unlock()

	switch f.Type {
	// Protocol messages
	case types.TypeSTP:
		p.router.state.Act(&p.reader, func() {
			if err := p.router.state._handleTreeAnnouncement(p, f); err != nil {
				p.router.log.Printf("Failed to handle tree announcement from %d: %s", p.port, err)
			}
		})

	case types.TypeKeepalive:
		// p.router.log.Println("Got keepalive")

	case types.TypeVirtualSnakeBootstrap:
		if deadend {
			p.router.state.Act(&p.reader, func() {
				if err := p.router.state._handleBootstrap(p, f); err != nil {
					p.router.log.Printf("Failed to handle SNEK bootstrap from %d: %s", p.port, err)
				}
			})
			nexthops = nil
		}

	case types.TypeVirtualSnakeBootstrapACK:
		if deadend {
			p.router.state.Act(&p.reader, func() {
				if err := p.router.state._handleBootstrapACK(p, f); err != nil {
					p.router.log.Printf("Failed to handle SNEK bootstrap ACK from %d: %s", p.port, err)
				}
			})
			nexthops = nil
		}

	case types.TypeVirtualSnakeSetup:
		p.router.state.Act(&p.reader, func() {
			if err := p.router.state._handleSetup(p, f); err != nil {
				p.router.log.Printf("Failed to handle SNEK setup from %d: %s", p.port, err)
			}
		})

	case types.TypeVirtualSnakeTeardown:
		p.router.state.Act(&p.reader, func() {
			if nexthops, err := p.router.state._handleTeardown(p, f); err != nil {
				p.router.log.Printf("Failed to handle SNEK teardown from %d: %s", p.port, err)
			} else {
				// Teardowns are a special case where we need to send to all
				// of the returned candidate ports, not just the first one that
				// we can.
				for _, peer := range nexthops {
					peer.send(f)
				}
			}
		})

	// Traffic messages
	case types.TypeVirtualSnake, types.TypeGreedy, types.TypeSource:

	case types.TypeSNEKPing:
		if f.DestinationKey == p.router.public {
			nexthops = nil
			p.traffic.push(&types.Frame{
				Type:           types.TypeSNEKPong,
				DestinationKey: f.SourceKey,
				SourceKey:      p.router.public,
			})
		}

	case types.TypeSNEKPong:
		if f.DestinationKey == p.router.public {
			nexthops = nil
			v, ok := p.router.pings.Load(f.SourceKey)
			if !ok {
				return nil
			}
			ch := v.(chan struct{})
			close(ch)
		}

	case types.TypeTreePing:
		if deadend {
			nexthops = nil
			p.traffic.push(&types.Frame{
				Type:        types.TypeTreePong,
				Destination: f.Source,
				Source:      p.router.state.coords(),
			})
		}

	case types.TypeTreePong:
		if deadend {
			nexthops = nil
			v, ok := p.router.pings.Load(f.Source.String())
			if !ok {
				return nil
			}
			ch := v.(chan struct{})
			close(ch)
		}

	}

	return nil
}

func (p *peer) _stop(err error) {
	if !p.started.CAS(true, false) {
		return
	}
	phony.Block(p.router, func() {
		p.cancel()
		for i, rp := range p.router._peers {
			if rp == p {
				p.router._peers[i] = nil
				if err != nil {
					p.router.log.Println("Disconnected from peer", p.public.String(), "on port", i, "due to error:", err)
				} else {
					p.router.log.Println("Disconnected from peer", p.public.String(), "on port", i)
				}
				// TODO: what makes more sense here?
				// p.router.state.Act(p.router, func() {
				phony.Block(p.router.state, func() {
					p.router.state._portDisconnected(p)
				})
				break
			}
		}
		index := hex.EncodeToString(p.public[:]) + p.zone
		if v, ok := p.router.active.Load(index); ok && v.(*atomic.Uint64).Dec() == 0 {
			p.router.active.Delete(index)
		}
	})
}

func (p *peer) _write() {
	var frame *types.Frame
	select {
	case <-p.context.Done():
		p._stop(nil)
		return
	case frame = <-p.proto.pop():
		p.proto.ack()
	case <-p.traffic.wait():
		frame, _ = p.traffic.pop()
	case <-time.After(PeerKeepaliveInterval):
		frame = &types.Frame{
			Type: types.TypeKeepalive,
		}
	}
	b := frameBufferPool.Get().(*[types.MaxFrameSize]byte)
	defer frameBufferPool.Put(b)
	buf := *b
	n, err := frame.MarshalBinary(buf[:])
	if err != nil {
		p._stop(fmt.Errorf("frame.MarshalBinary: %w", err))
		return
	}
	if err := p.conn.SetWriteDeadline(time.Now().Add(PeerKeepaliveInterval)); err != nil {
		p._stop(fmt.Errorf("p.conn.SetWriteDeadline: %w", err))
		return
	}
	wn, err := p.conn.Write(buf[:n])
	if err != nil {
		p._stop(fmt.Errorf("p.conn.Write: %w", err))
		return
	}
	if wn != n {
		p._stop(fmt.Errorf("p.conn.Write length %d != %d", wn, n))
		return
	}
	if err := p.conn.SetWriteDeadline(time.Time{}); err != nil {
		p._stop(fmt.Errorf("p.conn.SetWriteDeadline: %w", err))
		return
	}
	p.writer.Act(nil, p._write)
}

func (p *peer) _read() {
	select {
	case <-p.context.Done():
		p._stop(nil)
		return
	default:
	}
	b := frameBufferPool.Get().(*[types.MaxFrameSize]byte)
	defer frameBufferPool.Put(b)
	if err := p.conn.SetReadDeadline(time.Now().Add(PeerKeepaliveTimeout)); err != nil {
		p._stop(fmt.Errorf("p.conn.SetReadDeadline: %w", err))
		return
	}
	if _, err := io.ReadFull(p.conn, b[:8]); err != nil {
		p._stop(fmt.Errorf("io.ReadFull: %w", err))
		return
	}
	if !bytes.Equal(b[:4], types.FrameMagicBytes) {
		p._stop(fmt.Errorf("missing magic bytes"))
		return
	}
	expecting := int(binary.BigEndian.Uint16(b[6:8]))
	n, err := io.ReadFull(p.conn, b[8:expecting])
	if err != nil {
		p._stop(fmt.Errorf("io.ReadFull: %w", err))
		return
	}
	if err := p.conn.SetReadDeadline(time.Time{}); err != nil {
		p._stop(fmt.Errorf("conn.SetReadDeadline: %w", err))
		return
	}
	if n < expecting-8 {
		p._stop(fmt.Errorf("expecting %d bytes but got %d bytes", expecting, n))
		return
	}
	f := &types.Frame{
		Payload: make([]byte, 0, types.MaxPayloadSize),
	}
	if _, err := f.UnmarshalBinary(b[:n+8]); err != nil {
		p._stop(fmt.Errorf("f.UnmarshalBinary: %w", err))
		return
	}
	if err := p._receive(f); err != nil {
		p._stop(fmt.Errorf("p._receive: %w", err))
		return
	}
	p.reader.Act(nil, p._read)
}
