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

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/RyanCarrier/dijkstra"
)

type Simulator struct {
	log                      *log.Logger
	sockets                  bool
	ping                     bool
	nodes                    map[string]*Node
	nodesMutex               sync.RWMutex
	graph                    *dijkstra.Graph
	maps                     map[string]int
	wires                    map[string]map[string]net.Conn
	wiresMutex               sync.RWMutex
	dists                    map[string]map[string]*Distance
	distsMutex               sync.RWMutex
	snekPathConvergence      map[string]map[string]bool
	snekPathConvergenceMutex sync.RWMutex
	treePathConvergence      map[string]map[string]bool
	treePathConvergenceMutex sync.RWMutex
	startTime                time.Time
	State                    *StateAccessor
}

func NewSimulator(log *log.Logger, sockets, ping bool) *Simulator {
	sim := &Simulator{
		log:                 log,
		sockets:             sockets,
		ping:                ping,
		nodes:               make(map[string]*Node),
		wires:               make(map[string]map[string]net.Conn),
		dists:               make(map[string]map[string]*Distance),
		snekPathConvergence: make(map[string]map[string]bool),
		treePathConvergence: make(map[string]map[string]bool),
		startTime:           time.Now(),
		State:               NewStateAccessor(),
	}

	return sim
}

func (sim *Simulator) PingingEnabled() bool {
	return sim.ping
}

func (sim *Simulator) Nodes() map[string]*Node {
	sim.nodesMutex.RLock()
	defer sim.nodesMutex.RUnlock()
	return sim.nodes
}

func (sim *Simulator) Distances() map[string]map[string]*Distance {
	sim.distsMutex.RLock()
	defer sim.distsMutex.RUnlock()
	mapcopy := make(map[string]map[string]*Distance)
	for a, aa := range sim.dists {
		if _, ok := mapcopy[a]; !ok {
			mapcopy[a] = make(map[string]*Distance)
		}
		for b, bb := range aa {
			mapcopy[a][b] = bb
		}
	}
	return mapcopy
}

func (sim *Simulator) SNEKPathConvergence() map[string]map[string]bool {
	sim.snekPathConvergenceMutex.RLock()
	defer sim.snekPathConvergenceMutex.RUnlock()
	mapcopy := make(map[string]map[string]bool)
	for a, aa := range sim.snekPathConvergence {
		if _, ok := mapcopy[a]; !ok {
			mapcopy[a] = make(map[string]bool)
		}
		for b, bb := range aa {
			mapcopy[a][b] = bb
		}
	}
	return mapcopy
}

func (sim *Simulator) TreePathConvergence() map[string]map[string]bool {
	sim.treePathConvergenceMutex.RLock()
	defer sim.treePathConvergenceMutex.RUnlock()
	mapcopy := make(map[string]map[string]bool)
	for a, aa := range sim.treePathConvergence {
		if _, ok := mapcopy[a]; !ok {
			mapcopy[a] = make(map[string]bool)
		}
		for b, bb := range aa {
			mapcopy[a][b] = bb
		}
	}
	return mapcopy
}

func (sim *Simulator) Uptime() time.Duration {
	return time.Since(sim.startTime)
}

func (sim *Simulator) handlePeerAdded(node string, peerID string, port int) {
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		sim.State.Act(nil, func() { sim.State._addPeerConnection(node, peerNode, port) })
	}
}

func (sim *Simulator) handlePeerRemoved(node string, peerID string, port int) {
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		sim.State.Act(nil, func() { sim.State._removePeerConnection(node, peerNode, port) })
	}
}

func (sim *Simulator) handleTreeParentUpdate(node string, peerID string) {
	peerName := ""
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		peerName = peerNode
	}
	sim.State.Act(nil, func() { sim.State._updateParent(node, peerName) })
}

func (sim *Simulator) handleSnakeAscUpdate(node string, peerID string) {
	peerName := ""
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		peerName = peerNode
	}

	sim.State.Act(nil, func() { sim.State._updateAscendingPeer(node, peerName) })
}

func (sim *Simulator) handleSnakeDescUpdate(node string, peerID string) {
	peerName := ""
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		peerName = peerNode
	}
	sim.State.Act(nil, func() { sim.State._updateDescendingPeer(node, peerName) })
}

func (sim *Simulator) handleTreeRootAnnUpdate(node string, root string, sequence uint64, time uint64, coords []uint64) {
	rootName := ""
	if peerNode, err := sim.State.GetNodeName(root); err == nil {
		rootName = peerNode
	}
	sim.State.Act(nil, func() { sim.State._updateTreeRootAnnouncement(node, rootName, sequence, time, coords) })
}
