package router

import (
	"context"
	"fmt"
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
	reader struct {
		phony.Inbox
		// _announcement *types.SwitchAnnouncement
	}
	writer struct {
		phony.Inbox
	}
	router   *Router            // populated by router
	port     types.SwitchPortID // populated by router
	context  context.Context    // populated by router
	cancel   context.CancelFunc // populated by router
	conn     net.Conn           // populated by router
	zone     string             // populated by router
	peertype int                // populated by router
	public   types.PublicKey    // populated by router
	started  atomic.Bool        //
	proto    *fifoQueue         // thread-safe
	traffic  *lifoQueue         // thread-safe
}

func (p *peer) send(f *types.Frame) bool {
	switch f.Type {
	// Protocol messages
	case types.TypeSTP, types.TypeKeepalive:
		fallthrough
	case types.TypeDHTRequest, types.TypeDHTResponse:
		fallthrough
	case types.TypeVirtualSnakeBootstrap, types.TypeVirtualSnakeBootstrapACK:
		fallthrough
	case types.TypeVirtualSnakeSetup, types.TypeVirtualSnakeTeardown:
		return p.proto.push(f)

	// Traffic messages
	case types.TypeVirtualSnake, types.TypeGreedy, types.TypeSource:
		fallthrough
	case types.TypePathfind, types.TypeVirtualSnakePathfind:
		return p.traffic.push(f)

	// Unknown messages
	default:
		return false
	}
}

func (p *peer) _treeAnnouncement(f *types.Frame) error {
	return nil
}

func (p *peer) _receive(f *types.Frame) error {
	switch f.Type {
	// Protocol messages
	case types.TypeSTP:
		if err := p._treeAnnouncement(f); err != nil {
			p.router.log.Println("p._treeAnnouncement:", err)
		} else {
			p.router.state.Act(&p.reader, func() {
				p.router.state._handleTreeAnnouncement(f)
			})
		}

	case types.TypeKeepalive:

	case types.TypeVirtualSnakeBootstrap:
		p.router.state.Act(&p.reader, func() {
			p.router.state._handleBootstrap(f)
		})

	case types.TypeVirtualSnakeBootstrapACK:
		p.router.state.Act(&p.reader, func() {
			p.router.state._handleBootstrapACK(f)
		})

	case types.TypeVirtualSnakeSetup:
		p.router.state.Act(&p.reader, func() {
			p.router.state._handleSetup(f)
		})

	case types.TypeVirtualSnakeTeardown:
		p.router.state.Act(&p.reader, func() {
			p.router.state._handleTeardown(f)
		})

	// Traffic messages
	case types.TypeVirtualSnake, types.TypeGreedy, types.TypeSource:
		p.router.Act(&p.reader.Inbox, func() {
			for _, nexthop := range p.router.state.nextHopsFor(f) {
				if peer := p.router._peers[nexthop]; peer != nil {
					peer.send(f)
				}
			}
		})
	}

	return nil
}

func (p *peer) _stop(err error) {
	if !p.started.CAS(true, false) {
		return
	}
	if err != nil {
		p.router.log.Println("p._stop:", err)
	}
	phony.Block(p.router, func() {
		p.cancel()
		for i, rp := range p.router._peers {
			if rp == p {
				p.router._peers[i] = nil
				p.router.log.Println("Disconnected from peer", p.public.String(), "on port", i)
				return
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
	case <-p.traffic.wait():
		frame, _ = p.traffic.pop()
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
	n, err := p.conn.Read(b[:])
	if err != nil {
		p._stop(fmt.Errorf("p.conn.Read: %w", err))
		return
	}
	f := framePool.Get().(*types.Frame)
	f.Reset()
	if _, err := f.UnmarshalBinary(b[:n]); err != nil {
		p._stop(fmt.Errorf("f.UnmarshalBinary: %w", err))
		return
	}
	if err := p._receive(f); err != nil {
		p._stop(fmt.Errorf("p._receive: %w", err))
		return
	}
	p.reader.Act(nil, p._read)
}
