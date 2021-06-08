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

// getNextHops is called to determine the next-hop ports for a given
// packet. This function will determine the frame type and automatically
// call the correct function by routing scheme. We might end up using:
//  * greedy routing following the spanning tree, for routing to/from
//    spanning tree coordinates;
//  * source routing, where the path is already known ahead of time;
//  * virtual snake routing, for most traffic.
// This function also ensures that root announcements, bootstrap and
// path setup messages are processed as necessary.
func (p *Peer) getNextHops(frame *types.Frame, from types.SwitchPortID) types.SwitchPorts {
	switch frame.Type {
	case types.TypeSTP:
		if from != 0 {
			p.r.handleAnnouncement(p, frame)
		}

	case types.TypeVirtualSnakeBootstrap:
		nextHops := p.r.snake.getVirtualSnakeNextHop(p, frame.DestinationKey, true)
		if nextHops.EqualTo(types.SwitchPorts{0}) {
			if err := p.r.snake.handleBootstrap(p, frame); err != nil {
				p.r.log.Println("Failed to handle bootstrap:", err)
			}
		} else {
			return nextHops
		}

	case types.TypeVirtualSnakeBootstrapACK:
		nextHops := p.r.getGreedyRoutedNextHop(p, frame)
		if nextHops.EqualTo(types.SwitchPorts{0}) {
			if err := p.r.snake.handleBootstrapACK(p, frame); err != nil {
				p.r.log.Println("Failed to handle bootstrap ACK:", err)
			}
		} else {
			return nextHops
		}

	case types.TypeVirtualSnakeTeardown:
		return p.r.snake.getVirtualSnakeTeardownNextHop(p, frame)

	case types.TypeVirtualSnakeSetup:
		nextHops := p.r.getGreedyRoutedNextHop(p, frame)
		if err := p.r.snake.handleSetup(p, frame, nextHops); err == nil {
			return nextHops
		} else {
			p.r.log.Println("Failed to handle setup:", err)
		}

	case types.TypeVirtualSnake, types.TypeVirtualSnakePathfind:
		return p.r.snake.getVirtualSnakeNextHop(p, frame.DestinationKey, false)

	case types.TypeGreedy, types.TypePathfind, types.TypeDHTRequest, types.TypeDHTResponse:
		return p.r.getGreedyRoutedNextHop(p, frame)

	case types.TypeSource:
		if p.port != 0 {
			frame.UpdateSourceRoutedPath(p.port)
		}
		return p.r.getSourceRoutedNextHop(p, frame)
	}

	// If we've got to this point, the packet type is unknown and
	// we don't know what to do with it, so just drop it.
	return nil
}
