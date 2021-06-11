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
	"github.com/matrix-org/pinecone/types"
)

// getGreedyRoutedNextHop returns next-hops for the given frame.
// If the frame is destined for us locally then a single switch port 0
// will be returned, causing the packet to be handed over to the router
// (as the router is always connected to switch port 0). Otherwise, zero
// or one next-hop ports can be returned.
func (r *Router) getGreedyRoutedNextHop(from *Peer, rx *types.Frame) types.SwitchPorts {
	// If it's loopback then don't bother doing anything else.
	ourCoords := r.Coords()
	if rx.Destination.EqualTo(ourCoords) {
		return types.SwitchPorts{0}
	}

	// Work out how close our own coordinates are to the destination
	// message. This is important because we'll only forward a frame
	// to a peer that takes the message closer to the destination than
	// we are.
	ourDist := int64(ourCoords.DistanceTo(rx.Destination))
	if ourDist == 0 {
		// It's impossible to get closer so there's a pretty good
		// chance at this point that the traffic is destined for us.
		// Pass it up to the router.
		return types.SwitchPorts{0}
	}

	// Now work out which of our peers takes the message closer.
	bestPeer := types.SwitchPortID(0)
	bestDist := ourDist
	for _, p := range r.activePorts() {
		// Don't deliberately create routing loops by forwarding
		// to a node that doesn't share our root - the coordinate
		// system will be different.
		if p.port == from.port || !p.SeenCommonRootRecently() {
			continue
		}

		// Look up the coordinates of the peer, and the distance
		// across the tree to those coordinates.
		peerCoords := p.Coordinates()
		peerDist := int64(peerCoords.DistanceTo(rx.Destination))
		switch {
		case peerDist == 0:
			return []types.SwitchPortID{p.port}
		case rx.Destination.EqualTo(peerCoords):
			return []types.SwitchPortID{p.port}
		case peerDist < bestDist:
			bestPeer, bestDist = p.port, peerDist
		default:
		}
	}

	// If we've got an eligible next peer, and it doesn't create a
	// routing loop by sending the frame back where it came from,
	// then return it.
	return types.SwitchPorts{bestPeer}
}
