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
	"fmt"
	"net"
	"time"

	"github.com/matrix-org/pinecone/types"
)

// _nextHopsFor returns the next-hop for the given frame. It will examine the packet
// type and use the correct routing algorithm to determine the next-hop. It is possible
// for this function to return `nil` if there is no suitable candidate.
func (s *state) _nextHopsFor(from *peer, frameType types.FrameType, dest net.Addr, watermark types.VirtualSnakeWatermark) (*peer, types.VirtualSnakeWatermark) {
	var nexthop *peer
	switch frameType {
	case types.TypeBootstrap, types.TypeTrafficSNEK:
		if dest, ok := dest.(types.PublicKey); ok {
			nexthop, watermark = s._nextHopsSNEK(dest, frameType, watermark)
		}
	case types.TypeTrafficTree:
		if dest, ok := dest.(types.Coordinates); ok {
			nexthop = s._nextHopsTree(from, dest)
		}
	}
	return nexthop, watermark
}

// _forward handles frames received from a given peer. In most cases, this function will
// look up the best next-hop for a given frame and forward it to the appropriate peer
// queue if possible. In some special cases, like tree announcements,
// special handling will be done before forwarding if needed.
func (s *state) _forward(p *peer, f *types.Frame) error {
	// Allow overlay loopback traffic by directly forwarding it to the local router.
	if f.Type.IsTraffic() {
		switch {
		case f.DestinationKey == s.r.public:
			fallthrough
		case f.Type == types.TypeTrafficTree && f.Destination.EqualTo(s._coords()):
			if len(f.Source) > 0 {
				s._coordsCache[f.SourceKey] = coordsCacheEntry{
					coordinates: f.Source,
					lastSeen:    time.Now(),
				}
			}
			if !s.r.local.send(f) {
				framePool.Put(f)
			}
			return nil
		}
	}

	if s._filterPacket != nil && s._filterPacket(p.public, f) {
		s.r.log.Printf("Packet of type %s destined for port %d [%s] was dropped due to filter rules", f.Type.String(), p.port, p.public.String()[:8])
		framePool.Put(f)
		return nil
	}

	var nexthop *peer
	var watermark types.VirtualSnakeWatermark
	switch f.Type {
	case types.TypeTrafficTree:
		if nexthop, watermark = s._nextHopsFor(p, f.Type, f.Destination, f.Watermark); nexthop != nil {
			// We found a next-hop on the tree, so use it
			break
		}
		// Otherwise, we failed to find a tree next-hop, fall back to SNEK routing
		f.Type, f.Destination = types.TypeTrafficSNEK, f.Destination[:0]
		fallthrough
	case types.TypeTrafficSNEK, types.TypeBootstrap:
		nexthop, watermark = s._nextHopsFor(p, f.Type, f.DestinationKey, f.Watermark)
	}
	deadend := nexthop == nil || nexthop == p.router.local

	switch f.Type {
	case types.TypeTreeAnnouncement:
		// Tree announcements are a special case. The _handleTreeAnnouncement function
		// will generate new tree announcements and send them to peers if needed.
		defer framePool.Put(f)
		if err := s._handleTreeAnnouncement(p, f); err != nil {
			return fmt.Errorf("s._handleTreeAnnouncement (port %d): %w", p.port, err)
		}
		return nil

	case types.TypeKeepalive:
		// Keepalives are sent on a peering and are never forwarded.
		framePool.Put(f)
		return nil

	case types.TypeBootstrap:
		// Bootstrap messages are handled at each node along the path.
		if !s._handleBootstrap(p, nexthop, f) || deadend {
			framePool.Put(f)
			return nil
		}

	case types.TypeTrafficSNEK, types.TypeTrafficTree:
		// Traffic type packets are forwarded normally by falling through. There
		// are no special rules to apply to these packets, regardless of whether
		// they are SNEK-routed or tree-routed.
	}

	// If the packet's watermark is higher than the previous one or we are
	// obviously looping, drop the packet.
	// In the case of initial pong response frames, they are routed back to
	// the peer we received the ping from so the "loop" is desired.
	if nexthop == p || watermark.WorseThan(f.Watermark) {
		// s.r.log.Println("Dropping forwarded packet of type", f.Type)
		framePool.Put(f)
		return nil
	}

	// If there's a suitable next-hop then try sending the packet. If we fail
	// to queue up the packet then we will log it but there isn't an awful lot
	// we can do at this point.
	f.Watermark = watermark
	if nexthop != nil && !nexthop.send(f) {
		// s.r.log.Println("Dropping forwarded packet of type", f.Type)
		framePool.Put(f)
	}

	return nil
}
