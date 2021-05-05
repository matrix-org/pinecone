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
	"fmt"
	"net"
	"strings"

	"github.com/matrix-org/pinecone/types"
)

func (sim *Simulator) LookupCoords(target string) (types.SwitchPorts, error) {
	node, ok := sim.nodes[target]
	if !ok {
		return nil, fmt.Errorf("node %q not known", target)
	}
	return node.Coords(), nil
}

func (sim *Simulator) LookupNodeID(target types.SwitchPorts) (string, error) {
	for id, n := range sim.nodes {
		if n.Coords().EqualTo(target) {
			return id, nil
		}
	}
	return "", fmt.Errorf("coords %v not known", target)
}

func (sim *Simulator) LookupPublicKey(target types.PublicKey) (string, error) {
	for id, n := range sim.nodes {
		if n.PublicKey().EqualTo(target) {
			return id, nil
		}
	}
	return "", fmt.Errorf("public key %s not known", target.String())
}

func (sim *Simulator) ReportNewLink(c net.Conn, source, target types.PublicKey) {
	sourceID, err := sim.LookupPublicKey(source)
	if err != nil {
		return
	}
	if _, err = sim.LookupPublicKey(target); err == nil {
		return
	}
	sim.wiresMutex.Lock()
	defer sim.wiresMutex.Unlock()
	if _, ok := sim.wires[sourceID]; !ok {
		sim.wires[sourceID] = map[string]net.Conn{}
	}
	sim.wires[sourceID][target.String()] = c
}

func (sim *Simulator) ReportDeadLink(source, target types.PublicKey) {
	sourceID, err := sim.LookupPublicKey(source)
	if err != nil {
		return
	}
	if _, err = sim.LookupPublicKey(target); err == nil {
		return
	}
	sim.wiresMutex.Lock()
	defer sim.wiresMutex.Unlock()
	if _, ok := sim.wires[sourceID]; !ok {
		return
	}
	delete(sim.wires[sourceID], target.String())
}

func (sim *Simulator) ReportDistance(a, b string, l int64) {
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
