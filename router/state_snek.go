package router

import (
	"time"

	"github.com/matrix-org/pinecone/types"
)

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

type virtualSnakeNeighbour struct {
	PublicKey     types.PublicKey
	Port          types.SwitchPortID
	LastSeen      time.Time
	Coords        types.SwitchPorts
	PathID        types.VirtualSnakePathID
	RootPublicKey types.PublicKey
}

func (s *state) _nextHopsSNEK(from *peer, f *types.Frame) []*peer {
	return nil
}

func (s *state) _rootChanged(new types.PublicKey) {

}

func (s *state) _handleBootstrap(f *types.Frame) {

}

func (s *state) _handleBootstrapACK(f *types.Frame) {

}

func (s *state) _handleSetup(f *types.Frame) {

}

func (s *state) _handleTeardown(f *types.Frame) {

}
