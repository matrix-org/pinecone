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

type FloodType int

const (
	ClassicFlood FloodType = iota
	TreeFlood
)

// _nextHopsFor returns the next-hop for the given frame. It will examine the packet
// type and use the correct routing algorithm to determine the next-hop. It is possible
// for this function to return `nil` if there is no suitable candidate.
func (s *state) _nextHopsFor(from *peer, frameType types.FrameType, dest net.Addr, watermark types.VirtualSnakeWatermark) (*peer, types.VirtualSnakeWatermark) {
	var nexthop *peer
	switch dest := dest.(type) {
	case types.PublicKey:
		nexthop, watermark = s._nextHopsSNEK(dest, frameType, watermark)
	case types.Coordinates:
		nexthop = s._nextHopsTree(from, dest)
	}
	return nexthop, watermark
}

// _forward handles frames received from a given peer. In most cases, this function will
// look up the best next-hop for a given frame and forward it to the appropriate peer
// queue if possible. In some special cases, like tree announcements,
// special handling will be done before forwarding if needed.
func (s *state) _forward(p *peer, f *types.Frame) error {
	// Allow overlay loopback traffic by directly forwarding it to the local router.
	if f.Type.IsTraffic() && f.DestinationKey == s.r.public {
		if len(f.Source) > 0 {
			// TODO: There's a potential security risk here in that currently a node
			// on the path could modify the source coordinates and that would cause
			// return traffic to be redirected via a different route. The obvious
			// solution here is to "seal" the source key and coordinates in the packet
			// by encrypting them to resist changes or on-path statistical analysis.
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

	if s._filterPacket != nil && s._filterPacket(p.public, f) {
		s.r.log.Printf("Packet of type %s destined for port %d [%s] was dropped due to filter rules", f.Type.String(), p.port, p.public.String()[:8])
		framePool.Put(f)
		return nil
	}

	var nexthop *peer
	var watermark types.VirtualSnakeWatermark
	switch f.Type {
	case types.TypeTraffic:
		if len(f.Destination) > 0 {
			if nexthop, watermark = s._nextHopsFor(p, f.Type, f.Destination, f.Watermark); nexthop != nil {
				// We found a next-hop on the tree, so use it
				break
			}
		}
		// Otherwise, we failed to find a tree next-hop, fall back to SNEK routing
		f.Destination = f.Destination[:0]
		fallthrough
	case types.TypeBootstrap:
		nexthop, watermark = s._nextHopsFor(p, f.Type, f.DestinationKey, f.Watermark)
	}
	deadend := nexthop == nil || nexthop == p.router.local

	switch f.Type {
	case types.TypeKeepalive:
		// Keepalives are sent on a peering and are never forwarded.
		framePool.Put(f)
		return nil

	case types.TypeTreeAnnouncement:
		// Tree announcements are a special case. The _handleTreeAnnouncement function
		// will generate new tree announcements and send them to peers if needed.
		defer framePool.Put(f)
		if err := s._handleTreeAnnouncement(p, f); err != nil {
			return fmt.Errorf("s._handleTreeAnnouncement (port %d): %w", p.port, err)
		}
		return nil

	case types.TypeBootstrap:
		// Bootstrap messages are handled at each node along the path.
		if !s._handleBootstrap(p, nexthop, f) || deadend {
			framePool.Put(f)
			return nil
		}

	case types.TypeWakeupBroadcast:
		// Broadcasts are a special case. The _handleBroadcast function will handle
		// forwarding broadcasts as appropriate.
		if err := s._handleBroadcast(p, f); err != nil {
			return fmt.Errorf("s._handleBroadcast (port %d): %w", p.port, err)
		}
		return nil

	case types.TypeTraffic:
		// Traffic type packets are forwarded normally by falling through unless hop
		// limiting is enabled.
		if s.r._hopLimiting.Load() {
			if f.HopLimit > 1 {
				f.HopLimit -= 1
			} else {
				// The packet has reached the hop limit and shouldn't be forwarded.
				return nil
			}
		}

	default:
		// We don't know what type of packet this is so drop it.
		return nil
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

// _flood sends a frame to all of our connected peers. This is used for
// flooding the wakeup broadcast to all of our direct peers.
// Classic flooding works by sending frames to all other peers.
// Tree flooding works by only sending frames to peers on the same branch.
func (s *state) _flood(from *peer, f *types.Frame, floodType FloodType) {
	floodCandidates := make(map[types.PublicKey]*peer)
	for _, newCandidate := range s._peers {
		if newCandidate == nil || newCandidate.proto == nil || !newCandidate.started.Load() {
			continue
		}

		if newCandidate == from || newCandidate == s.r.local {
			continue
		}

		if s._filterPacket != nil && s._filterPacket(newCandidate.public, f) {
			s.r.log.Printf("Packet of type %s destined for port %d [%s] was dropped due to filter rules", f.Type.String(), newCandidate.port, newCandidate.public.String()[:8])
			continue
		}

		if floodType == TreeFlood {
			if coords, err := newCandidate._coords(); err == nil {
				if coords.DistanceTo(s._coords()) != 1 {
					// This peer is not directly on the same branch.
					continue
				}
			} else {
				continue
			}
		}

		if existingCandidate, ok := floodCandidates[newCandidate.public]; ok {
			fasterNewPeerType := newCandidate.peertype < existingCandidate.peertype
			lowerLatencyNewCandidate := false
			if existingAnnouncement, ok := s._announcements[existingCandidate]; ok {
				if newAnnouncement, ok := s._announcements[newCandidate]; ok {
					if newAnnouncement.receiveOrder < existingAnnouncement.receiveOrder {
						lowerLatencyNewCandidate = true
					}
				}
			}

			betterCandidate := fasterNewPeerType || lowerLatencyNewCandidate
			if !betterCandidate {
				continue
			}
		}

		floodCandidates[newCandidate.public] = newCandidate
	}

	for _, p := range floodCandidates {
		frame := getFrame()
		f.CopyInto(frame)
		p.send(frame)
	}
}
