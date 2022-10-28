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

package simulator

import "github.com/matrix-org/pinecone/router/events"

type SimEvent interface {
	isEvent()
}

type NodeAdded struct {
	Node       string
	PublicKey  string
	NodeType   int
	RouteCount int
}

// Tag NodeAdded as an Event
func (e NodeAdded) isEvent() {}

type NodeRemoved struct {
	Node string
}

// Tag NodeRemoved as an Event
func (e NodeRemoved) isEvent() {}

type PeerAdded struct {
	Node string
	Peer string
	Port uint64
}

// Tag PeerAdded as an Event
func (e PeerAdded) isEvent() {}

type PeerRemoved struct {
	Node string
	Peer string
}

// Tag PeerRemoved as an Event
func (e PeerRemoved) isEvent() {}

type TreeParentUpdate struct {
	Node string
	Peer string
	Prev string
}

// Tag TreeParentUpdate as an Event
func (e TreeParentUpdate) isEvent() {}

type SnakeAscUpdate struct {
	Node   string
	Peer   string
	Prev   string
	PathID string
}

// Tag SnakeAscUpdate as an Event
func (e SnakeAscUpdate) isEvent() {}

type SnakeDescUpdate struct {
	Node   string
	Peer   string
	Prev   string
	PathID string
}

// Tag SnakeDescUpdate as an Event
func (e SnakeDescUpdate) isEvent() {}

type TreeRootAnnUpdate struct {
	Node     string
	Root     string // Root Public Key
	Sequence uint64
	Time     uint64 // Unix Time
	Coords   []uint64
}

// Tag TreeRootAnnUpdate as an Event
func (e TreeRootAnnUpdate) isEvent() {}

type PingStateUpdate struct {
	Enabled bool
	Active  bool
}

// Tag PingStateUpdate as an Event
func (e PingStateUpdate) isEvent() {}

type NetworkStatsUpdate struct {
	PathConvergence uint64
	AverageStretch  float64
}

// Tag NetworkStatsUpdate as an Event
func (e NetworkStatsUpdate) isEvent() {}

type SnakeEntryAdded struct {
	Node    string
	EntryID string
	PeerID  string
}

// Tag SnakeEntryAdded as an Event
func (e SnakeEntryAdded) isEvent() {}

type SnakeEntryRemoved struct {
	Node    string
	EntryID string
}

// Tag SnakeEntryRemoved as an Event
func (e SnakeEntryRemoved) isEvent() {}

type BroadcastReceived struct {
	Node   string
	PeerID string
}

// Tag BroadcastReceived as an Event
func (e BroadcastReceived) isEvent() {}

type BandwidthReport struct {
	Node      string
	Bandwidth BandwidthSnapshot
}

// Tag BandwidthReport as an Event
func (e BandwidthReport) isEvent() {}

type eventHandler struct {
	node string
	ch   <-chan events.Event
}

func (h eventHandler) Run(quit <-chan bool, sim *Simulator) {
	for {
		select {
		case <-quit:
			return
		case event := <-h.ch:
			switch e := event.(type) {
			case events.PeerAdded:
				sim.handlePeerAdded(h.node, e.PeerID, int(e.Port))
			case events.PeerRemoved:
				sim.handlePeerRemoved(h.node, e.PeerID, int(e.Port))
			case events.TreeParentUpdate:
				sim.handleTreeParentUpdate(h.node, e.PeerID)
			case events.SnakeDescUpdate:
				sim.handleSnakeDescUpdate(h.node, e.PeerID, "") // TODO: do we need the path ID?
			case events.TreeRootAnnUpdate:
				sim.handleTreeRootAnnUpdate(h.node, e.Root, e.Sequence, e.Time, e.Coords)
			case events.SnakeEntryAdded:
				sim.handleSnakeEntryAdded(h.node, e.EntryID, e.PeerID)
			case events.SnakeEntryRemoved:
				sim.handleSnakeEntryRemoved(h.node, e.EntryID)
			case events.BroadcastReceived:
				sim.handleBroadcastReceived(h.node, e.PeerID)
			case events.BandwidthReport:
				sim.handleBandwidthReport(h.node, e.CaptureTime, e.Peers)
			default:
				sim.log.Println("Unhandled event!")
			}
		}
	}
}
