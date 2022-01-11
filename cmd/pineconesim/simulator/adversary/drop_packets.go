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

type FrameDropRates map[types.FrameType]uint64

type DropRates struct {
	Overall uint64
	Frames  FrameDropRates
}

func NewDropRates() DropRates {
	return DropRates{
		Overall: 0,
		Frames:  make(FrameDropRates, 9),
	}
}

type PeerDropRates map[types.PublicKey]DropRates

type DropSettings struct {
	overall DropRates
	peers   PeerDropRates
}

func NewDropSettings() DropSettings {
	return DropSettings{
		overall: DropRates{},
		peers:   make(PeerDropRates, 1),
	}
}

type PeerDropCounts PeerDropRates

type DropCounts PeerFrameCount

type PacketsDropped PacketsReceived

type FrameCounts map[types.FrameType]*atomic.Uint64

type PeerFrameCount struct {
	overall    *atomic.Uint64
	frameCount FrameCounts
}

type PeerFrameCounts map[types.PublicKey]PeerFrameCount

type PacketsReceived struct {
	overall *atomic.Uint64
	peers   PeerFrameCounts
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
	rtr            *router.Router
	dropSettings   DropSettings
	packetsRx      PacketsReceived
	packetsDropped PacketsDropped
}

func NewAdversaryRouter(log *log.Logger, sk ed25519.PrivateKey, debug bool) *AdversaryRouter {
	rtr := router.NewRouter(log, sk, debug)
	adversary := &AdversaryRouter{
		rtr,
		NewDropSettings(),
		NewPacketsReceived(),
		PacketsDropped(NewPacketsReceived()),
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
	a.dropSettings.overall = rates
}

func (a *AdversaryRouter) ConfigureFilterPeer(peer types.PublicKey, rates DropRates) {
	a.dropSettings.peers[peer] = rates
}

func (a *AdversaryRouter) updatePacketCounts(from types.PublicKey, frameType types.FrameType) {
	a.packetsRx.overall.Inc()
	a.packetsRx.peers[from].overall.Inc()
	a.packetsRx.peers[from].frameCount[frameType].Inc()
}

func (a *AdversaryRouter) updateDropCount(from types.PublicKey, f types.FrameType, countLimitOverall uint64, countLimitFrame uint64) {
	a.packetsDropped.overall.Inc()
	a.packetsDropped.peers[from].overall.Inc()
	a.packetsDropped.peers[from].frameCount[f].Inc()

	if a.packetsDropped.peers[from].overall.Load() >= countLimitOverall {
		a.packetsDropped.peers[from].overall.Store(0)
	}

	if a.packetsDropped.peers[from].frameCount[f].Load() >= countLimitFrame {
		a.packetsDropped.peers[from].frameCount[f].Store(0)
	}
}

func getHighestCommonFactor(num1 uint64, num2 uint64) uint64 {
	temp := uint64(0)

	for num2 != 0 {
		temp = num1 % num2
		num1 = num2
		num2 = temp
	}

	return num1
}

func (a *AdversaryRouter) selectivelyDrop(from types.PublicKey, f *types.Frame) bool {
	shouldDrop := false
	if _, containsPeer := a.packetsRx.peers[from]; !containsPeer {
		a.packetsRx.peers[from] = defaultFrameCount()
	}

	if _, containsPeer := a.packetsDropped.peers[from]; !containsPeer {
		a.packetsDropped.peers[from] = defaultFrameCount()
	}

	a.updatePacketCounts(from, f.Type)

	var dropRates DropRates
	if val, exists := a.dropSettings.peers[from]; exists {
		dropRates = val
	} else {
		dropRates = a.dropSettings.overall
	}

	// NOTE : Determine the HCF from the percentage provided and use that to drop
	// the first X / Y packets.

	hcfOverall := getHighestCommonFactor(dropRates.Overall, uint64(100))
	numberToDropOverall := dropRates.Overall / hcfOverall
	numberToCountOverall := 100 / hcfOverall

	hcfFrame := getHighestCommonFactor(dropRates.Frames[f.Type], uint64(100))
	numberToDropFrame := dropRates.Frames[f.Type] / hcfFrame
	numberToCountFrame := 100 / hcfFrame

	shouldDropOverall := a.packetsDropped.peers[from].overall.Load() < numberToDropOverall
	shouldDropFrame := a.packetsDropped.peers[from].frameCount[f.Type].Load() < numberToDropFrame
	if shouldDropOverall || shouldDropFrame {
		shouldDrop = true
	}

	a.updateDropCount(from, f.Type, numberToCountOverall, numberToCountFrame)

	if shouldDrop {
		log.Printf("Router %s :: Dropping %s", a.PublicKey().String()[:8], f.Type)
	}
	return shouldDrop
}
