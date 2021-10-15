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
	"net"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
)

// newLocalPeer returns a new local peer. It should only be called once when
// the router is set up.
func (r *Router) newLocalPeer() *peer {
	peer := &peer{
		router:   r,
		port:     0,
		context:  r.context,
		cancel:   r.cancel,
		conn:     nil,
		zone:     "local",
		peertype: 0,
		public:   r.public,
		traffic:  newLIFOQueue(trafficBuffer),
	}
	return peer
}

// ReadFrom reads the next packet that was delivered to this node over the
// Pinecone network. Only traffic frames will be returned here (not protocol
// frames). The returned address will either be a `types.PublicKey` (if the
// frame was delivered using SNEK routing) or `types.Coordinates` (if the frame
// was delivered using tree routing).
func (r *Router) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	var frame *types.Frame
	select {
	case <-r.local.context.Done():
		r.local.stop(nil)
		return
	case <-r.local.traffic.wait():
		frame, _ = r.local.traffic.pop()
	}
	switch frame.Type {
	case types.TypeTreeRouted:
		addr = frame.Source

	case types.TypeVirtualSnakeRouted:
		addr = frame.SourceKey

	default:
		return
	}

	n = len(frame.Payload)
	copy(p, frame.Payload)
	return
}

// WriteTo sends a packet into the Pinecone network. The packet will be sent
// as a traffic packet. The supplied net.Addr will dictate the method used to
// route the packet — the address should be a `types.PublicKey` for SNEK routing
// or `types.Coordinates` for tree routing. Supplying an unsupported address type
// will result in a `*net.AddrError` being returned.
func (r *Router) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	timer := time.NewTimer(time.Second * 5)
	defer func() {
		if !timer.Stop() {
			<-timer.C
		}
	}()

	switch ga := addr.(type) {
	case types.Coordinates:
		phony.Block(r.state, func() {
			frame := getFrame()
			frame.Type = types.TypeTreeRouted
			frame.Destination = append(frame.Destination[:0], ga...)
			frame.Source = append(frame.Source[:0], r.state.coords()...)
			frame.Payload = append(frame.Payload[:0], p...)
			_ = r.state._forward(r.local, frame)
		})
		return len(p), nil

	case types.PublicKey:
		phony.Block(r.state, func() {
			frame := getFrame()
			frame.Type = types.TypeVirtualSnakeRouted
			frame.DestinationKey = ga
			frame.SourceKey = r.public
			frame.Payload = append(frame.Payload[:0], p...)
			_ = r.state._forward(r.local, frame)
		})
		return len(p), nil

	default:
		err = &net.AddrError{
			Err:  "unexpected address type",
			Addr: addr.String(),
		}
		return
	}
}

// LocalAddr returns a net.Addr containing the public key of the node for
// SNEK routing.
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
