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
}

func NewSimulator(log *log.Logger) *Simulator {
	sim := &Simulator{
		log:                 log,
		nodes:               make(map[string]*Node),
		wires:               make(map[string]map[string]net.Conn),
		dists:               make(map[string]map[string]*Distance),
		snekPathConvergence: make(map[string]map[string]bool),
		treePathConvergence: make(map[string]map[string]bool),
		startTime:           time.Now(),
	}
	return sim
}

func (sim *Simulator) Nodes() map[string]*Node {
	sim.nodesMutex.RLock()
	defer sim.nodesMutex.RUnlock()
	return sim.nodes
}

func (sim *Simulator) Wires() map[string]map[string]net.Conn {
	sim.wiresMutex.RLock()
	defer sim.wiresMutex.RUnlock()
	return sim.wires
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
