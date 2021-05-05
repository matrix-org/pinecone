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

import "github.com/matrix-org/pinecone/types"

const (
	routerPort types.SwitchPortID = 0
)

// getSourceRoutedNextHop returns next-hops for the given frame.
// If the frame is destined for us locally then a single switch port 0
// will be returned, causing the packet to be handed over to the router
// (as the router is always connected to switch port 0). Otherwise, zero
// or one next-hop ports can be returned.
func (r *Router) getSourceRoutedNextHop(from *Peer, rx *types.Frame) types.SwitchPorts {
	to := types.SwitchPortID(0)
	if len(rx.Destination) > 0 {
		// The packet has another destination port, so the
		// next-hop is basically the next port in the list.
		to = types.SwitchPortID(rx.Destination[0])
	} else {
		// The packet has no more destination ports so it is
		// probably for us locally.
		return types.SwitchPorts{0}
	}

	if from.port == routerPort && rx.Type == types.TypeSTP {
		// Only allow broadcast frames if they came from the
		// local router and they are spanning-tree maintenance
		// frames.
		return r.getSourceRoutedBroadcastNextHops()
	}

	if from.port == to {
		// Don't allow a packet to be sent back from where it
		// came from. This probably means there's been a loop
		// somewhere.
		return nil
	}

	if peer := r.ports[to]; !peer.started.Load() || !peer.alive.Load() {
		// Don't try to send packets to a port that has nothing
		// connected to it or isn't alive.
		return nil
	}

	return []types.SwitchPortID{to}
}

// getSourceRoutedBroadcastNextHops returns a list of next-hops for
// all active switch ports. This is used for spanning tree root
// announcements.
func (r *Router) getSourceRoutedBroadcastNextHops() (peers types.SwitchPorts) {
	for _, peer := range r.ports {
		if peer.started.Load() {
			peers = append(peers, peer.port)
		}
	}
	return
}
