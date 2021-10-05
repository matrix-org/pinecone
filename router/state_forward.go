package router

import (
	"fmt"

	"github.com/matrix-org/pinecone/types"
)

func (s *state) _forward(p *peer, f *types.Frame) error {
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
		if err := p.router.state._handleSetup(p, f, nexthop); err != nil {
			return fmt.Errorf("p.router.state._handleSetup (port %d): %s", p.port, err)
		}

	case types.TypeVirtualSnakeTeardown:
		if nexthop, err := p.router.state._handleTeardown(p, f); err == nil {
			// Teardowns are a special case where we need to send to all
			// of the returned candidate ports, not just the first one that
			// we can.
			if nexthop != nil {
				nexthop.send(f)
			}
		} else {
			return fmt.Errorf("p.router.state._handleTeardown (port %d): %s", p.port, err)
		}
		return nil

	// Traffic messages
	case types.TypeVirtualSnake, types.TypeGreedy, types.TypeSource:

	case types.TypeSNEKPing:
		if f.DestinationKey == p.router.public {
			p.traffic.push(&types.Frame{
				Type:           types.TypeSNEKPong,
				DestinationKey: f.SourceKey,
				SourceKey:      p.router.public,
			})
			return nil
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
			nexthop = nil
			p.traffic.push(&types.Frame{
				Type:        types.TypeTreePong,
				Destination: f.Source,
				Source:      p.router.state.coords(),
			})
		}

	case types.TypeTreePong:
		if deadend {
			nexthop = nil
			v, ok := p.router.pings.Load(f.Source.String())
			if !ok {
				return nil
			}
			ch := v.(chan struct{})
			close(ch)
		}
	}

	if p := nexthop; p != nil {
		p.send(f)
		return nil
	}

	return fmt.Errorf("no next-hop found for packet of type %s", f.Type)
}
