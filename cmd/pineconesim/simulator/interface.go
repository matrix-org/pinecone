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
	"math"
	"strings"
)

func (sim *Simulator) ReportDistance(a, b string, l int64, snek bool) {
	// TODO: why is this 0 compare here?
	if strings.Compare(a, b) > 0 {
		a, b = b, a
	}

	sim.distsMutex.Lock()
	defer sim.distsMutex.Unlock()

	if _, ok := sim.dists[a]; !ok {
		sim.dists[a] = map[string]*Distance{}
	}

	if _, ok := sim.dists[a][b]; !ok {
		sim.dists[a][b] = &Distance{}
	}

	if snek {
		sim.dists[a][b].ObservedSNEK = l
	} else {
		sim.dists[a][b].ObservedTree = l
	}
}

func (sim *Simulator) UpdateRealDistances() {
	sim.distsMutex.Lock()
	defer sim.distsMutex.Unlock()

	for from := range sim.nodes {
		for to := range sim.nodes {
			if _, ok := sim.dists[from]; !ok {
				sim.dists[from] = map[string]*Distance{}
			}

			if _, ok := sim.dists[from][to]; !ok {
				sim.dists[from][to] = &Distance{}
			}

			a, _ := sim.graph.GetMapping(from)
			b, _ := sim.graph.GetMapping(to)
			if a != -1 && b != -1 {
				path, err := sim.graph.Shortest(a, b)

				if err == nil {
					sim.dists[from][to].Real = path.Distance
				} else {
					sim.dists[from][to].Real = math.MaxInt64
				}
			}
		}
	}

	sim.State.Act(nil, func() {
		sim.distsMutex.Lock()
		defer sim.distsMutex.Unlock()
		sim.State._updateExpectedBroadcasts(sim.dists)
	})
}
