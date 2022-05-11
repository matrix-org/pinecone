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

	"github.com/matrix-org/pinecone/types"
)

// _nextHopsFor returns the next-hop for the given frame. It will examine the packet
// type and use the correct routing algorithm to determine the next-hop. It is possible
// for this function to return `nil` if there is no suitable candidate.
func (s *state) _nextHopsFor(from *peer, frameType types.FrameType, dest net.Addr) *peer {
	var nexthop *peer
	switch frameType {
	case types.TypeVirtualSnakeTeardown:
		// Teardowns have their own logic so we do nothing with them
		return nil

	case types.TypeVirtualSnakeSetupACK:
		// Setup ACKs have their own logic so we do nothing with them
		return nil

	// SNEK routing
	case types.TypeVirtualSnakeRouted, types.TypeVirtualSnakeBootstrap:
		switch dest := (dest).(type) {
		case types.PublicKey:
			nexthop = s._nextHopsSNEK(dest, frameType)
		}

	// Tree routing
	case types.TypeTreeRouted, types.TypeVirtualSnakeBootstrapACK, types.TypeVirtualSnakeSetup:
		switch dest := (dest).(type) {
		case types.Coordinates:
			nexthop = s._nextHopsTree(from, dest)
		}
	}
	return nexthop
}

// _forward handles frames received from a given peer. In most cases, this function will
// look up the best next-hop for a given frame and forward it to the appropriate peer
// queue if possible. In some special cases, like tree announcements, path setups and
// teardowns, special handling will be done before forwarding if needed.
func (s *state) _forward(p *peer, f *types.Frame) error {
	if s._filterPacket != nil && s._filterPacket(p.public, f) {
		s.r.log.Printf("Packet of type %s destined for port %d [%s] was dropped due to filter rules", f.Type.String(), p.port, p.public.String()[:8])
		return nil
	}

	var nexthop *peer
	switch f.Type {
	case types.TypeTreeRouted, types.TypeVirtualSnakeBootstrapACK, types.TypeVirtualSnakeSetup:
		nexthop = s._nextHopsFor(p, f.Type, net.Addr(f.Destination))
	case types.TypeVirtualSnakeBootstrap, types.TypeVirtualSnakeRouted, types.TypeVirtualSnakeTeardown:
		nexthop = s._nextHopsFor(p, f.Type, net.Addr(f.DestinationKey))
	}
	deadend := nexthop == p.router.local

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
		if deadend {
			if err := s._handleBootstrap(p, f); err != nil {
				return fmt.Errorf("s._handleBootstrap (port %d): %w", p.port, err)
			}
			return nil
		}

	case types.TypeVirtualSnakeBootstrapACK:
		// Bootstrap ACK messages are only handled specially when they reach a dead end.
		// Otherwise they are forwarded normally by falling through.
		if deadend {
			if err := s._handleBootstrapACK(p, f); err != nil {
				return fmt.Errorf("s._handleBootstrapACK (port %d): %w", p.port, err)
			}
			return nil
		}

	case types.TypeVirtualSnakeSetup:
		// Setup messages are handled at each node on the path. Since the _handleSetup
		// function needs to be sure that the setup message was queued to the next-hop
		// before installing the route, we do not need to forward the packet here.
		if err := s._handleSetup(p, f, nexthop); err != nil {
			return fmt.Errorf("s._handleSetup (port %d): %w", p.port, err)
		}
		return nil

	case types.TypeVirtualSnakeSetupACK:
		// Setup ACK messages are handled at each node on the path. Since the _handleSetupACK
		// function needs to be sure that the setup ACK message was queued to the next-hop
		// before activating the route, we do not need to forward the packet here.
		if err := s._handleSetupACK(p, f, nexthop); err != nil {
			return fmt.Errorf("s._handleSetupACK (port %d): %w", p.port, err)
		}
		return nil

	case types.TypeVirtualSnakeTeardown:
		// Teardown messages are a special case where there might be more than one
		// next-hop, so this is handled specifically.
		if nexthops, err := s._handleTeardown(p, f); err != nil {
			return fmt.Errorf("s._handleTeardown (port %d): %w", p.port, err)
		} else {
			for _, nexthop := range nexthops {
				if nexthop != nil && nexthop.proto != nil {
					nexthop.proto.push(f)
				}
			}
		}
		return nil

	case types.TypeVirtualSnakeRouted, types.TypeTreeRouted:
		// Traffic type packets are forwarded normally by falling through. There
		// are no special rules to apply to these packets, regardless of whether
		// they are SNEK-routed or tree-routed.
	}

	// If there's a suitable next-hop then try sending the packet. If we fail
	// to queue up the packet then we will log it but there isn't an awful lot
	// we can do at this point.
	if nexthop != nil && !nexthop.send(f) {
		s.r.log.Println("Dropping forwarded packet of type", f.Type)
	}

	return nil
}
