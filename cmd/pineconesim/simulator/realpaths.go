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
	"github.com/RyanCarrier/dijkstra"
)

func (sim *Simulator) CalculateShortestPaths(nodes map[string]struct{}, wires map[string]map[string]bool) {
	sim.log.Println("Building graph")
	sim.graph = dijkstra.NewGraph()
	sim.maps = make(map[string]int)
	for n := range nodes {
		sim.maps[n] = sim.graph.AddMappedVertex(n)
	}
	for a, aa := range wires {
		for b := range aa {
			if err := sim.graph.AddMappedArc(a, b, 1); err != nil {
				panic(err)
			}
			if err := sim.graph.AddMappedArc(b, a, 1); err != nil {
				panic(err)
			}
		}
	}
}
