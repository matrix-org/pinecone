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

func (sim *Simulator) ReportDistance(a, b string, l int64) {
	sim.distsMutex.Lock()
	defer sim.distsMutex.Unlock()
	if _, ok := sim.dists[a]; !ok {
		sim.dists[a] = map[string]*Distance{}
	}
	if _, ok := sim.dists[a][b]; !ok {
		sim.dists[a][b] = &Distance{}
	}
	sim.dists[a][b].Observed = l
	if sim.dists[a][b].Real == 0 {
		na, _ := sim.graph.GetMapping(a)
		nb, _ := sim.graph.GetMapping(b)
		path, err := sim.graph.Shortest(na, nb)
		if err == nil {
			sim.dists[a][b].Real = path.Distance
		}
	}
}
