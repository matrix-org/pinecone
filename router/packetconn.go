package router

import (
	"fmt"
	"net"
	"time"

	"github.com/matrix-org/pinecone/types"
)

// GreedyAddr implements net.Addr, containing a greedy-routed
// set of destination coordinates to another node.
type GreedyAddr struct {
	types.SwitchPorts
}

func (a GreedyAddr) Network() string {
	return "pg"
}

func (a GreedyAddr) String() string {
	return fmt.Sprintf("coords %v", a.SwitchPorts)
}

// ReadFrom reads the next packet that was delivered to this
// node over the Pinecone network. Only traffic packets will
// be returned here - no protocol messages will be included.
// The net.Addr returned will contain the appropriate return
// path based on the mechanism used to deliver the packet.
// If the packet was delivered using greedy routing, then the
// net.Addr will contain the source coordinates. If the packet
// was delivered using source routing, then the net.Addr will
// contain the source-routed path back to the sender.
func (r *Router) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	/*
		frame := <-r.recv
		switch frame.Type {
		case types.TypeGreedy:
			addr = GreedyAddr{frame.Source}

		//case types.TypeSource:
		//	addr = SourceAddr{frame.Source} // TODO: should get the remainder of the path

		case types.TypeVirtualSnakeBootstrap:
			addr = frame.SourceKey

		case types.TypeVirtualSnake:
			addr = frame.SourceKey

		default:
			r.log.Println("Not expecting non-source/non-greedy frame")
			return
		}

		n = len(frame.Payload)
		copy(p, frame.Payload)
	*/
	return
}

// WriteTo sends a packet into the Pinecone network. The
// packet will be sent as a traffic packet. The net.Addr must
// be one of the Pinecone address types (e.g. GreedyAddr or
// SourceAddr), as this will dictate the method of delivery
// used to forward the packet.
func (r *Router) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	/*
		timer := time.NewTimer(time.Second * 5)
		defer func() {
			if !timer.Stop() {
				<-timer.C
			}
		}()

		switch ga := addr.(type) {
		case GreedyAddr:
			select {
			case <-timer.C:
				return 0, fmt.Errorf("router appears to be deadlocked")
			case r.send <- types.Frame{
				Version:     types.Version0,
				Type:        types.TypeGreedy,
				Destination: ga.SwitchPorts,
				Source:      r.Coords(),
				Payload:     append([]byte{}, p...),
			}:
				return len(p), nil
			}

		case SourceAddr:
			select {
			case <-timer.C:
				return 0, fmt.Errorf("router appears to be deadlocked")
			case r.send <- types.Frame{
				Version:     types.Version0,
				Type:        types.TypeSource,
				Destination: ga.SwitchPorts,
				Payload:     append([]byte{}, p...),
			}:
				return len(p), nil
			}

		case types.PublicKey:
			select {
			case <-timer.C:
				return 0, fmt.Errorf("router appears to be deadlocked")
			case r.send <- types.Frame{
				Version:        types.Version0,
				Type:           types.TypeVirtualSnake,
				DestinationKey: ga,
				SourceKey:      r.PublicKey(),
				Payload:        append([]byte{}, p...),
			}:
				return len(p), nil
			}

		default:
			err = fmt.Errorf("unknown address type")
			return
		}
	*/
	return
}

// LocalAddr returns a net.Addr containing the greedy routing
// coordinates for this node.
func (r *Router) LocalAddr() net.Addr {
	return r.PublicKey()
}

// SetDeadline is not implemented.
func (r *Router) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline is not implemented.
func (r *Router) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline is not implemented.
func (r *Router) SetWriteDeadline(t time.Time) error {
	return nil
}
