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
	"math"

	"github.com/matrix-org/pinecone/types"
)

// getGreedyRoutedNextHop returns next-hops for the given frame.
// If the frame is destined for us locally then a single switch port 0
// will be returned, causing the packet to be handed over to the router
// (as the router is always connected to switch port 0). Otherwise, zero
// or one next-hop ports can be returned.
func (r *Router) getGreedyRoutedNextHop(from *Peer, rx *types.Frame) types.SwitchPorts {
	// If it's loopback then don't bother doing anything else.
	if rx.Destination.EqualTo(r.Coords()) {
		return types.SwitchPorts{0}
	}

	// Work out how close our own coordinates are to the destination
	// message. This is important because we'll only forward a frame
	// to a peer that takes the message closer to the destination than
	// we are.
	ourDist := int64(r.Coords().DistanceTo(rx.Destination))
	if ourDist == 0 {
		// It's impossible to get closer so there's a pretty good
		// chance at this point that the traffic is destined for us.
		// Pass it up to the router.
		return types.SwitchPorts{0}
	}

	// Now work out which of our peers takes the message closer.
	bestPeer := types.SwitchPortID(0)
	bestDist := int64(math.MaxInt64)
	for _, p := range r.activePorts() {
		// Don't create routing loops.
		if p.port == from.port {
			continue
		}

		// Look up the coordinates of the peer.
		p.mutex.RLock()
		coords := p.coords
		p.mutex.RUnlock()

		// Work out what the distance across the tree is to that
		// peer.
		dist := int64(coords.DistanceTo(rx.Destination))

		// If the distance is zero, that's because the peer is the
		// destination itself.
		if dist == 0 {
			return []types.SwitchPortID{p.port}
		}

		// Otherwise, let's see if this peer just happens to be a
		// better candidate for the next-hop.
		switch {
		case dist > ourDist:
			// TODO: is this needed?
			// This peer will take the traffic further away from our
			// own node.
		case dist > bestDist:
			// This isn't any closer to the destination than our
			// current best candidate next-hop.
		default:
			// This looks like probably the best next-hop candidate we
			// have so far.
			bestPeer, bestDist = p.port, dist
		}
	}

	// If we've got an eligible next peer, and it doesn't create a
	// routing loop by sending the frame back where it came from,
	// then return it.
	peers := types.SwitchPorts{}
	if bestPeer != 0 {
		peers = append(peers, bestPeer)
	}
	return peers
}
