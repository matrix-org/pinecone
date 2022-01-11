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

package adversary

import (
	"context"
	"crypto/ed25519"
	"log"
	"net"
	"time"

	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/router/events"
	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

type DropRates struct {
	KeepAlive                int
	TreeAnnouncement         int
	TreeRouted               int
	VirtualSnakeBootstrap    int
	VirtualSnakeBootstrapACK int
	VirtualSnakeSetup        int
	VirtualSnakeSetupACK     int
	VirtualSnakeTeardown     int
	VirtualSnakeRouted       int
}

type FrameCounts map[types.FrameType]*atomic.Uint64

type PeerFrameCount struct {
	overall    *atomic.Uint64
	frameCount FrameCounts
}

type PeerFrameCounts map[types.PublicKey]PeerFrameCount

type PacketsReceived struct {
	peers   PeerFrameCounts
	overall *atomic.Uint64
}

func NewPacketsReceived() PacketsReceived {
	return PacketsReceived{
		peers:   make(PeerFrameCounts, 1),
		overall: atomic.NewUint64(0),
	}
}

func defaultFrameCount() PeerFrameCount {
	frameCount := make(FrameCounts, 9)
	frameCount[types.TypeKeepalive] = atomic.NewUint64(0)
	frameCount[types.TypeTreeAnnouncement] = atomic.NewUint64(0)
	frameCount[types.TypeVirtualSnakeBootstrap] = atomic.NewUint64(0)
	frameCount[types.TypeVirtualSnakeBootstrapACK] = atomic.NewUint64(0)
	frameCount[types.TypeVirtualSnakeSetup] = atomic.NewUint64(0)
	frameCount[types.TypeVirtualSnakeSetupACK] = atomic.NewUint64(0)
	frameCount[types.TypeVirtualSnakeTeardown] = atomic.NewUint64(0)
	frameCount[types.TypeTreeRouted] = atomic.NewUint64(0)
	frameCount[types.TypeVirtualSnakeRouted] = atomic.NewUint64(0)

	peerFrameCount := PeerFrameCount{
		frameCount: frameCount,
		overall:    atomic.NewUint64(0),
	}

	return peerFrameCount
}

type AdversaryRouter struct {
	rtr       *router.Router
	packetsRx PacketsReceived
}

func NewAdversaryRouter(log *log.Logger, sk ed25519.PrivateKey, debug bool) *AdversaryRouter {
	rtr := router.NewRouter(log, sk, debug)
	adversary := &AdversaryRouter{
		rtr,
		NewPacketsReceived(),
	}

	rtr.InjectPacketFilter(adversary.selectivelyDrop)
	return adversary
}

func (a *AdversaryRouter) Subscribe(ch chan events.Event) {
	a.rtr.Subscribe(ch)
}

func (a *AdversaryRouter) PublicKey() types.PublicKey {
	return a.rtr.PublicKey()
}

func (a *AdversaryRouter) Connect(conn net.Conn, options ...router.ConnectionOption) (types.SwitchPortID, error) {
	return a.rtr.Connect(conn, options...)
}

func (a *AdversaryRouter) Ping(ctx context.Context, addr net.Addr) (uint16, time.Duration, error) {
	return a.rtr.Ping(ctx, addr)
}

func (a *AdversaryRouter) Coords() types.Coordinates {
	return a.rtr.Coords()
}

func (a *AdversaryRouter) ConfigureFilterDefaults(rates DropRates) {
	log.Println("New Default Rates")
	// TODO : me
}

func (a *AdversaryRouter) ConfigureFilterPeer(peer types.PublicKey, rates DropRates) {
	log.Println("New Peer Rates")
	// TODO : me
}

func (a *AdversaryRouter) updatePacketCounts(from types.PublicKey, f *types.Frame) {
	a.packetsRx.overall.Inc()
	val, containsPeer := a.packetsRx.peers[from]
	if containsPeer {
		val.overall.Inc()
		val.frameCount[f.Type].Inc()
	} else {
		a.packetsRx.peers[from] = defaultFrameCount()
	}
}

func (a *AdversaryRouter) selectivelyDrop(from types.PublicKey, f *types.Frame) bool {
	shouldDrop := false
	a.updatePacketCounts(from, f)
	log.Printf("Router %s :: Received %s", a.PublicKey(), f.Type)

	// TODO : drop appropriate packets
	// TODO : reset appropriate packet counts

	switch f.Type {
	case types.TypeKeepalive:
		shouldDrop = false
	case types.TypeTreeAnnouncement:
		shouldDrop = false
	case types.TypeVirtualSnakeBootstrap:
		shouldDrop = false
	case types.TypeVirtualSnakeBootstrapACK:
		shouldDrop = false
	case types.TypeVirtualSnakeSetup:
		shouldDrop = false
	case types.TypeVirtualSnakeSetupACK:
		shouldDrop = false
	case types.TypeVirtualSnakeTeardown:
		shouldDrop = false
	case types.TypeVirtualSnakeRouted, types.TypeTreeRouted:
		shouldDrop = false
	case types.TypeSNEKPing:
		shouldDrop = false
	case types.TypeSNEKPong:
		shouldDrop = false
	case types.TypeTreePing:
		shouldDrop = false
	case types.TypeTreePong:
		shouldDrop = false
	}

	return shouldDrop
}
