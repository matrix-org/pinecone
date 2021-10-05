package router

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
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

func (p *peer) String() string { // to make sim less ugly
	if p == nil {
		return "nil"
	}
	if p.port == 0 {
		return "local"
	}
	return fmt.Sprintf("%d", p.port)
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
		if p.proto == nil {
			// The local peer doesn't have a protocol queue so we should check
			// for nils to prevent panics.
			return true
		}
		return p.proto.push(f)

	// Traffic messages
	case types.TypeVirtualSnake, types.TypeGreedy, types.TypeSource:
		fallthrough
	case types.TypeSNEKPing, types.TypeSNEKPong, types.TypeTreePing, types.TypeTreePong:
		return p.traffic.push(f)
	}

	return false
}

func (p *peer) _receive(f *types.Frame) error {
	nexthops := p.router.state.nextHopsFor(p, f)
	deadend := len(nexthops) == 0 || nexthops[0] == p.router.local
	defer func() {
		if len(nexthops) == 0 {
			return
		}
		for _, peer := range nexthops {
			if peer.send(f) {
				return
			}
		}
		p.router.log.Println("Dropping traffic frame", f.Type, "due to congestion - next-hops were", nexthops)
	}()

	switch f.Type {
	// Protocol messages
	case types.TypeSTP:
		p.router.state.Act(nil, func() {
			if err := p.router.state._handleTreeAnnouncement(p, f); err != nil {
				p.router.log.Printf("Failed to handle tree announcement from %d: %s", p.port, err)
			}
		})

	case types.TypeKeepalive:
		// p.router.log.Println("Got keepalive")

	case types.TypeVirtualSnakeBootstrap:
		if deadend {
			p.router.state.Act(nil, func() {
				if err := p.router.state._handleBootstrap(p, f); err != nil {
					p.router.log.Printf("Failed to handle SNEK bootstrap from %d: %s", p.port, err)
				}
			})
			nexthops = nil
		}

	case types.TypeVirtualSnakeBootstrapACK:
		if deadend {
			p.router.state.Act(nil, func() {
				if err := p.router.state._handleBootstrapACK(p, f); err != nil {
					p.router.log.Printf("Failed to handle SNEK bootstrap ACK from %d: %s", p.port, err)
				}
			})
			nexthops = nil
		}

	case types.TypeVirtualSnakeSetup:
		p.router.state.Act(nil, func() {
			if err := p.router.state._handleSetup(p, f, nexthops); err != nil {
				p.router.log.Printf("Failed to handle SNEK setup from %d: %s", p.port, err)
			}
		})

	case types.TypeVirtualSnakeTeardown:
		p.router.state.Act(nil, func() {
			if nexthop, err := p.router.state._handleTeardown(p, f); err == nil {
				// Teardowns are a special case where we need to send to all
				// of the returned candidate ports, not just the first one that
				// we can.
				if nexthop != nil {
					nexthop.send(f)
				}
			} else {
				p.router.log.Printf("Failed to handle SNEK teardown from %d: %s", p.port, err)
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
	// The atomic switch here makes sure that the port won't be used
	// instantly. Then we'll cancel the context and reduce the connection
	// count.
	if !p.started.CAS(true, false) {
		return
	}
	p.cancel()
	index := hex.EncodeToString(p.public[:]) + p.zone
	if v, ok := p.router.active.Load(index); ok && v.(*atomic.Uint64).Dec() == 0 {
		p.router.active.Delete(index)
	}
	// Next we'll send a message to the state inbox asking it to clean
	// up the port.
	p.router.state.Act(nil, func() {
		p.router.state._portDisconnected(p)

		for i, rp := range p.router.state._peers {
			if rp == p {
				rp.proto.reset()
				rp.traffic.reset()
				p.router.state._peers[i] = nil
				if err != nil {
					p.router.log.Println("Disconnected from peer", p.public.String(), "on port", i, "due to error:", err)
				} else {
					p.router.log.Println("Disconnected from peer", p.public.String(), "on port", i)
				}
				break
			}
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
	default:
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
	}
	if frame == nil {
		// usually happens if the queue has been reset
		return
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
