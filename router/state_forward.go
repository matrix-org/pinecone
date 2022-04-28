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
	"encoding/binary"
	"fmt"

	"github.com/matrix-org/pinecone/types"
)

// _nextHopsFor returns the next-hop for the given frame. It will examine the packet
// type and use the correct routing algorithm to determine the next-hop. It is possible
// for this function to return `nil` if there is no suitable candidate.
func (s *state) _nextHopsFor(from *peer, frame *types.Frame) (*peer, types.VirtualSnakeWatermark) {
	var nexthop *peer
	var watermark types.VirtualSnakeWatermark
	switch frame.Type {
	// SNEK routing
	case types.TypeVirtualSnakeRouted, types.TypeVirtualSnakeBootstrap, types.TypeSNEKPing, types.TypeSNEKPong:
		nexthop, watermark = s._nextHopsSNEK(from, frame)

	// Tree routing
	case types.TypeTreeRouted, types.TypeTreePing, types.TypeTreePong:
		nexthop = s._nextHopsTree(from, frame)
	}
	return nexthop, watermark
}

// _forward handles frames received from a given peer. In most cases, this function will
// look up the best next-hop for a given frame and forward it to the appropriate peer
// queue if possible. In some special cases, like tree announcements,
// special handling will be done before forwarding if needed.
func (s *state) _forward(p *peer, f *types.Frame) error {
	if s._filterPacket != nil && s._filterPacket(p.public, f) {
		s.r.log.Printf("Packet of type %s destined for port %d [%s] was dropped due to filter rules", f.Type.String(), p.port, p.public.String()[:8])
		return nil
	}

	nexthop, watermark := s._nextHopsFor(p, f)
	deadend := nexthop == nil || nexthop == p.router.local

	isInitialPongResponse := false

	switch f.Type {
	case types.TypeTreeAnnouncement:
		// Tree announcements are a special case. The _handleTreeAnnouncement function
		// will generate new tree announcements and send them to peers if needed.
		if err := s._handleTreeAnnouncement(p, f); err != nil {
			return fmt.Errorf("s._handleTreeAnnouncement (port %d): %w", p.port, err)
		}
		return nil

	case types.TypeKeepalive:
		// Keepalives are sent on a peering and are never forwarded.
		return nil

	case types.TypeVirtualSnakeBootstrap:
		// Bootstrap messages are only handled specially when they reach a dead end.
		// Otherwise they are forwarded normally by falling through.
		if !s._handleBootstrap(p, nexthop, f) || deadend {
			return nil
		}

	case types.TypeVirtualSnakeRouted, types.TypeTreeRouted:
		// Traffic type packets are forwarded normally by falling through. There
		// are no special rules to apply to these packets, regardless of whether
		// they are SNEK-routed or tree-routed.

	case types.TypeSNEKPing:
		if f.DestinationKey == s.r.public {
			isInitialPongResponse = true
			of := f
			defer framePool.Put(of)
			f = getFrame()
			f.Type = types.TypeSNEKPong
			f.DestinationKey = of.SourceKey
			f.SourceKey = s.r.public
			f.Extra = of.Extra
			f.Watermark = types.VirtualSnakeWatermark{
				PublicKey: types.FullMask,
				Sequence:  0,
			}
			nexthop, watermark = s._nextHopsFor(s.r.local, f)
		} else {
			hops := binary.BigEndian.Uint16(f.Extra[:])
			hops++
			binary.BigEndian.PutUint16(f.Extra[:], hops)
		}

	case types.TypeSNEKPong:
		if f.DestinationKey == s.r.public {
			id := f.SourceKey.String()
			v, ok := s.r.pings.Load(id)
			if !ok {
				return nil
			}
			ch := v.(chan uint16)
			ch <- binary.BigEndian.Uint16(f.Extra[:])
			close(ch)
			s.r.pings.Delete(id)
			return nil
		}

	case types.TypeTreePing:
		if deadend {
			isInitialPongResponse = true
			of := f
			defer framePool.Put(of)
			f = getFrame()
			f.Type = types.TypeTreePong
			f.Destination = append(f.Destination[:0], of.Source...)
			f.Source = append(f.Source[:0], s._coords()...)
			f.Extra = of.Extra
			nexthop, watermark = s._nextHopsFor(s.r.local, f)
		} else {
			hops := binary.BigEndian.Uint16(f.Extra[:])
			hops++
			binary.BigEndian.PutUint16(f.Extra[:], hops)
		}

	case types.TypeTreePong:
		if deadend {
			id := f.Source.String()
			v, ok := s.r.pings.Load(id)
			if !ok {
				return nil
			}
			ch := v.(chan uint16)
			ch <- binary.BigEndian.Uint16(f.Extra[:])
			close(ch)
			s.r.pings.Delete(id)
			return nil
		}
	}

	// If the packet's watermark is higher than the previous one or we are
	// obviously looping, drop the packet.
	// In the case of initial pong response frames, they are routed back to
	// the peer we received the ping from so the "loop" is desired.
	if (nexthop == p && !isInitialPongResponse) || watermark.WorseThan(f.Watermark) {
		return nil
	}

	// If there's a suitable next-hop then try sending the packet. If we fail
	// to queue up the packet then we will log it but there isn't an awful lot
	// we can do at this point.
	if watermark.Sequence > 0 {
		f.Watermark = watermark
	}
	if nexthop != nil && !nexthop.send(f) {
		s.r.log.Println("Dropping forwarded packet of type", f.Type)
	}

	return nil
}
