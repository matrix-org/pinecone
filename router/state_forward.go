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

	"github.com/matrix-org/pinecone/types"
)

func (s *state) _nextHopsFor(from *peer, frame *types.Frame) *peer {
	var nexthop *peer
	switch frame.Type {
	case types.TypeVirtualSnakeTeardown:
		// Teardowns have their own logic so we do nothing with them
		return nil

	// SNEK routing
	case types.TypeVirtualSnake, types.TypeVirtualSnakeBootstrap, types.TypeSNEKPing, types.TypeSNEKPong:
		nexthop = s._nextHopsSNEK(from, frame, frame.Type == types.TypeVirtualSnakeBootstrap)

	// Tree routing
	case types.TypeGreedy, types.TypeVirtualSnakeBootstrapACK, types.TypeVirtualSnakeSetup, types.TypeTreePing, types.TypeTreePong:
		nexthop = s._nextHopsTree(from, frame)

	// Source routing
	case types.TypeSource:
		if len(frame.Destination) == 0 {
			return s.r.local
		}
		var nexthop *peer
		port := s._peers[frame.Destination[0]]
		if frame.Destination[0] == from.port {
			return nil
		}
		frame.Destination = frame.Destination[1:]
		if from != nexthop && nexthop != nil && nexthop.started.Load() {
			nexthop = port
		}
		if nexthop != nil {
			return nexthop
		}
	}
	return nexthop
}

func (s *state) _forward(p *peer, f *types.Frame) error {
	nexthop := s._nextHopsFor(p, f)
	deadend := nexthop == nil || nexthop == p.router.local

	switch f.Type {
	// Protocol messages
	case types.TypeSTP:
		if err := s._handleTreeAnnouncement(p, f); err != nil {
			return fmt.Errorf("s._handleTreeAnnouncement (port %d): %s", p.port, err)
		}
		return nil

	case types.TypeKeepalive:
		return nil

	case types.TypeVirtualSnakeBootstrap:
		if deadend {
			if err := s._handleBootstrap(p, f); err != nil {
				return fmt.Errorf("s._handleBootstrap (port %d): %s", p.port, err)
			}
			return nil
		}

	case types.TypeVirtualSnakeBootstrapACK:
		if deadend {
			if err := s._handleBootstrapACK(p, f); err != nil {
				return fmt.Errorf("s._handleBootstrapACK (port %d): %s", p.port, err)
			}
			return nil
		}

	case types.TypeVirtualSnakeSetup:
		if err := s._handleSetup(p, f, nexthop); err != nil {
			return fmt.Errorf("s._handleSetup (port %d): %s", p.port, err)
		}
		return nil

	case types.TypeVirtualSnakeTeardown:
		if nexthops, err := s._handleTeardown(p, f); err != nil {
			return fmt.Errorf("s._handleTeardown (port %d): %s", p.port, err)
		} else {
			for _, nexthop := range nexthops {
				if nexthop != nil && nexthop.proto != nil {
					nexthop.proto.push(f)
				}
			}
		}
		return nil

	// Traffic messages
	case types.TypeVirtualSnake, types.TypeGreedy, types.TypeSource:

	case types.TypeSNEKPing:
		if f.DestinationKey == s.r.public {
			of := f
			defer framePool.Put(of)
			f = getFrame()
			f.Type = types.TypeSNEKPong
			f.DestinationKey = of.SourceKey
			f.SourceKey = s.r.public
			nexthop = s._nextHopsFor(s.r.local, f)
		}

	case types.TypeSNEKPong:
		if f.DestinationKey == s.r.public {
			v, ok := s.r.pings.Load(f.SourceKey)
			if !ok {
				return nil
			}
			ch := v.(chan struct{})
			close(ch)
			s.r.pings.Delete(f.SourceKey)
			return nil
		}

	case types.TypeTreePing:
		if deadend {
			of := f
			defer framePool.Put(of)
			f = getFrame()
			f.Type = types.TypeTreePong
			f.Destination = append(f.Destination[:0], of.Source...)
			f.Source = append(f.Source[:0], s._coords()...)
			nexthop = s._nextHopsFor(s.r.local, f)
		}

	case types.TypeTreePong:
		if deadend {
			v, ok := s.r.pings.Load(f.Source.String())
			if !ok {
				return nil
			}
			ch := v.(chan struct{})
			close(ch)
			s.r.pings.Delete(f.Source.String())
			return nil
		}
	}

	if nexthop != nil && !nexthop.send(f) {
		s.r.log.Println("Dropping forwarded packet of type", f.Type)
	}

	return nil
}
