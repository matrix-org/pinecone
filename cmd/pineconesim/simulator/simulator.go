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

	"github.com/RyanCarrier/dijkstra"
)

type Simulator struct {
	log                  *log.Logger
	nodes                map[string]*Node
	nodesMutex           sync.RWMutex
	graph                *dijkstra.Graph
	maps                 map[string]int
	wires                map[string]map[string]net.Conn
	wiresMutex           sync.RWMutex
	dists                map[string]map[string]*Distance
	distsMutex           sync.RWMutex
	pathConvergence      map[string]map[string]bool
	pathConvergenceMutex sync.RWMutex
	dhtConvergence       map[string]map[string]bool
	dhtConvergenceMutex  sync.RWMutex
}

func NewSimulator(log *log.Logger) *Simulator {
	sim := &Simulator{
		log:             log,
		nodes:           make(map[string]*Node),
		wires:           make(map[string]map[string]net.Conn),
		dists:           make(map[string]map[string]*Distance),
		pathConvergence: make(map[string]map[string]bool),
		dhtConvergence:  make(map[string]map[string]bool),
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
	return sim.dists
}

func (sim *Simulator) PathConvergence() map[string]map[string]bool {
	sim.pathConvergenceMutex.RLock()
	defer sim.pathConvergenceMutex.RUnlock()
	return sim.pathConvergence
}

func (sim *Simulator) DHTConvergence() map[string]map[string]bool {
	sim.dhtConvergenceMutex.RLock()
	defer sim.dhtConvergenceMutex.RUnlock()
	return sim.dhtConvergence
}
