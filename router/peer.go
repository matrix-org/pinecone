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
const PeerKeepaliveTimeout = time.Second * 5

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

func (p *peer) stop(err error) {
	// The atomic switch here immediately makes sure that the port won't be
	// used. Then we'll cancel the context and reduce the connection count.
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
		p.stop(nil)
		return
	case frame = <-p.proto.pop():
		p.proto.ack()
	default:
		select {
		case <-p.context.Done():
			p.stop(nil)
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
		p.stop(fmt.Errorf("frame.MarshalBinary: %w", err))
		return
	}
	if err := p.conn.SetWriteDeadline(time.Now().Add(PeerKeepaliveInterval)); err != nil {
		p.stop(fmt.Errorf("p.conn.SetWriteDeadline: %w", err))
		return
	}
	wn, err := p.conn.Write(buf[:n])
	if err != nil {
		p.stop(fmt.Errorf("p.conn.Write: %w", err))
		return
	}
	if wn != n {
		p.stop(fmt.Errorf("p.conn.Write length %d != %d", wn, n))
		return
	}
	if err := p.conn.SetWriteDeadline(time.Time{}); err != nil {
		p.stop(fmt.Errorf("p.conn.SetWriteDeadline: %w", err))
		return
	}
	p.writer.Act(nil, p._write)
}

func (p *peer) _read() {
	select {
	case <-p.context.Done():
		p.stop(nil)
		return
	default:
	}
	b := frameBufferPool.Get().(*[types.MaxFrameSize]byte)
	defer frameBufferPool.Put(b)
	if err := p.conn.SetReadDeadline(time.Now().Add(PeerKeepaliveTimeout)); err != nil {
		p.stop(fmt.Errorf("p.conn.SetReadDeadline: %w", err))
		return
	}
	if _, err := io.ReadFull(p.conn, b[:types.FrameHeaderLength]); err != nil {
		p.stop(fmt.Errorf("io.ReadFull: %w", err))
		return
	}
	if !bytes.Equal(b[:4], types.FrameMagicBytes) {
		p.stop(fmt.Errorf("missing magic bytes"))
		return
	}
	expecting := int(binary.BigEndian.Uint16(b[types.FrameHeaderLength-2 : types.FrameHeaderLength]))
	n, err := io.ReadFull(p.conn, b[types.FrameHeaderLength:expecting])
	if err != nil {
		p.stop(fmt.Errorf("io.ReadFull: %w", err))
		return
	}
	if err := p.conn.SetReadDeadline(time.Time{}); err != nil {
		p.stop(fmt.Errorf("conn.SetReadDeadline: %w", err))
		return
	}
	if n < expecting-types.FrameHeaderLength {
		p.stop(fmt.Errorf("expecting %d bytes but got %d bytes", expecting, n))
		return
	}
	f := &types.Frame{
		Payload: make([]byte, 0, types.MaxPayloadSize),
	}
	if _, err := f.UnmarshalBinary(b[:n+types.FrameHeaderLength]); err != nil {
		p.stop(fmt.Errorf("f.UnmarshalBinary: %w", err))
		return
	}
	p.router.state.Act(&p.reader, func() {
		if err := p.router.state._forward(p, f); err != nil {
			p.router.log.Println("Error handling packet:", err)
		}
	})
	p.reader.Act(nil, p._read)
}
