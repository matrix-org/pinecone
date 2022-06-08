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

	"github.com/matrix-org/pinecone/types"
)

// _nextHopsFor returns the next-hop for the given frame. It will examine the packet
// type and use the correct routing algorithm to determine the next-hop. It is possible
// for this function to return `nil` if there is no suitable candidate.
func (s *state) _nextHopsFor(from *peer, frameType types.FrameType, dest net.Addr, watermark types.VirtualSnakeWatermark) (*peer, types.VirtualSnakeWatermark) {
	var nexthop *peer
	var newWatermark types.VirtualSnakeWatermark
	switch frameType {
	// SNEK routing
	case types.TypeVirtualSnakeRouted, types.TypeVirtualSnakeBootstrap:
		switch dest := (dest).(type) {
		case types.PublicKey:
			nexthop, newWatermark = s._nextHopsSNEK(dest, frameType, watermark)
		}
	}
	return nexthop, newWatermark
}

// _flood sends a frame to all of our connected peers. This is typically used for
// flooding the bootstrap for the highest key to all of our direct peers.
func (s *state) _flood(from *peer, f *types.Frame) {
	for _, p := range s._peers {
		if p == nil || p.proto == nil || !p.started.Load() {
			continue
		}
		if p == from || p == s.r.local {
			continue
		}
		if s._filterPacket != nil && s._filterPacket(p.public, f) {
			s.r.log.Printf("Packet of type %s destined for port %d [%s] was dropped due to filter rules", f.Type.String(), p.port, p.public.String()[:8])
			continue
		}
		frame := getFrame()
		f.CopyInto(frame)
		p.send(frame)
	}
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

	// Allow overlay loopback traffic by directly forwarding it to the local router.
	isSnakeLoopback := f.Type == types.TypeVirtualSnakeRouted && f.DestinationKey == s.r.public
	if isSnakeLoopback {
		s.r.local.send(f)
		return nil
	}

	var nexthop *peer
	var watermark types.VirtualSnakeWatermark
	switch f.Type {
	case types.TypeVirtualSnakeBootstrap, types.TypeVirtualSnakeRouted:
		nexthop, watermark = s._nextHopsFor(p, f.Type, f.DestinationKey, f.Watermark)
	}
	deadend := nexthop == nil || nexthop == p.router.local

	switch f.Type {
	case types.TypeKeepalive:
		// Keepalives are sent on a peering and are never forwarded.
		return nil

	case types.TypeVirtualSnakeBootstrap:
		// Bootstrap messages are handled at each node along the path.
		if !s._handleBootstrap(p, nexthop, f) || deadend {
			return nil
		}

	case types.TypeVirtualSnakeRouted:
		// Traffic type packets are forwarded normally by falling through. There
		// are no special rules to apply to these packets, regardless of whether
		// they are SNEK-routed or tree-routed.
	}

	// If the packet's watermark is higher than the previous one or we are
	// obviously looping, drop the packet.
	// In the case of initial pong response frames, they are routed back to
	// the peer we received the ping from so the "loop" is desired.
	if nexthop == p || watermark.WorseThan(f.Watermark) {
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
