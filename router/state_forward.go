package router

import (
	"fmt"

	"github.com/matrix-org/pinecone/types"
)

func (s *state) _forward(p *peer, f *types.Frame) error {
	var response *types.Frame
	nexthop := p.router.state._nextHopsFor(p, f)
	deadend := nexthop == nil || nexthop == p.router.local

	switch f.Type {
	// Protocol messages
	case types.TypeSTP:
		if err := p.router.state._handleTreeAnnouncement(p, f); err != nil {
			return fmt.Errorf("p.router.state._handleTreeAnnouncement (port %d): %s", p.port, err)
		}
		return nil

	case types.TypeKeepalive:
		return nil

	case types.TypeVirtualSnakeBootstrap:
		if deadend {
			if err := p.router.state._handleBootstrap(p, f); err != nil {
				return fmt.Errorf("p.router.state._handleBootstrap (port %d): %s", p.port, err)
			}
			return nil
		}

	case types.TypeVirtualSnakeBootstrapACK:
		if deadend {
			if err := p.router.state._handleBootstrapACK(p, f); err != nil {
				return fmt.Errorf("p.router.state._handleBootstrapACK (port %d): %s", p.port, err)
			}
			return nil
		}

	case types.TypeVirtualSnakeSetup:
		if forward, err := p.router.state._handleSetup(p, f, nexthop); err != nil {
			return fmt.Errorf("p.router.state._handleSetup (port %d): %s", p.port, err)
		} else if !forward {
			return nil
		}

	case types.TypeVirtualSnakeTeardown:
		var err error
		if nexthop, err = p.router.state._handleTeardown(p, f); err != nil {
			return fmt.Errorf("p.router.state._handleTeardown (port %d): %s", p.port, err)
		}
		if nexthop == nil {
			return nil
		}

	// Traffic messages
	case types.TypeVirtualSnake, types.TypeGreedy, types.TypeSource:

	case types.TypeSNEKPing:
		if f.DestinationKey == p.router.public {
			response = &types.Frame{
				Type:           types.TypeSNEKPong,
				DestinationKey: f.SourceKey,
				SourceKey:      p.router.public,
			}
		}

	case types.TypeSNEKPong:
		if f.DestinationKey == p.router.public {
			v, ok := p.router.pings.Load(f.SourceKey)
			if !ok {
				return nil
			}
			ch := v.(chan struct{})
			close(ch)
			return nil
		}

	case types.TypeTreePing:
		if deadend {
			response = &types.Frame{
				Type:        types.TypeTreePong,
				Destination: f.Source,
				Source:      p.router.state._coords(),
			}
		}

	case types.TypeTreePong:
		if deadend {
			v, ok := p.router.pings.Load(f.Source.String())
			if !ok {
				return nil
			}
			ch := v.(chan struct{})
			close(ch)
			return nil
		}
	}

	if response != nil {
		if nexthop = p.router.state._nextHopsFor(s.r.local, response); nexthop != s.r.local {
			if !nexthop.traffic.push(response) {
				return fmt.Errorf("dropping response packet of type %s", f.Type)
			}
		}
		return nil
	}

	if p := nexthop; p != nil {
		if !p.send(f) {
			return fmt.Errorf("dropping forwarded packet of type %s", f.Type)
		}
		return nil
	}

	return fmt.Errorf("no next-hop found for packet of type %s", f.Type)
}
