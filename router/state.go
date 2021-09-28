package router

import (
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
)

type state struct {
	phony.Inbox
	r           *Router
	_ascending  bool
	_descending bool
	_parent     *peer
	_table      virtualSnakeTable
}

type virtualSnakeTable map[virtualSnakeIndex]virtualSnakeEntry

type virtualSnakeIndex struct {
	PublicKey types.PublicKey
	PathID    types.VirtualSnakePathID
}

type virtualSnakeEntry struct {
	SourcePort      types.SwitchPortID
	DestinationPort types.SwitchPortID
	LastSeen        time.Time
	RootPublicKey   types.PublicKey
}

func (s *state) nextHopsFor(f *types.Frame) types.SwitchPorts {
	var nexthops types.SwitchPorts
	switch f.Type {
	// SNEK routing
	case types.TypeVirtualSnakeBootstrap:
		fallthrough
	case types.TypeGreedy:
		phony.Block(s, func() {
			nexthops = s._nextHopsTree(f)
		})

	// Tree routing
	case types.TypeVirtualSnakeBootstrapACK, types.TypeVirtualSnakeSetup:
		fallthrough
	case types.TypeVirtualSnake:
		phony.Block(s, func() {
			nexthops = s._nextHopsSNEK(f)
		})
	}
	return nexthops
}
