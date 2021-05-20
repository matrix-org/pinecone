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
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/pinecone/router"
)

func (sim *Simulator) Pathfind(from, to string) error {
	fromnode := sim.nodes[from]
	tonode := sim.nodes[to]

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	dhtSuccess := false
	pathfindSuccess := false

	//sim.log.Println("DHT search from", from, "to", to)
	//public := tonode.PublicKey()

	defer func() {
		sim.pathConvergenceMutex.Lock()
		if _, ok := sim.pathConvergence[from]; !ok {
			sim.pathConvergence[from] = map[string]bool{}
		}
		sim.pathConvergence[from][to] = pathfindSuccess
		sim.pathConvergenceMutex.Unlock()

		sim.dhtConvergenceMutex.Lock()
		if _, ok := sim.dhtConvergence[from]; !ok {
			sim.dhtConvergence[from] = map[string]bool{}
		}
		sim.dhtConvergence[from][to] = dhtSuccess
		sim.dhtConvergenceMutex.Unlock()
	}()

	/*if _, _, err := fromnode.DHTSearch(ctx, public[:], false); err != nil {
		sim.log.Println("DHT search from", from, "to", to, "failed:", err)
	} else {
		dhtSuccess = true
	}*/
	//_, err := fromnode.Pathfind(ctx, tonode.PublicKey())
	_, err := fromnode.Pathfind(ctx, router.GreedyAddr{SwitchPorts: tonode.Coords()})
	if err != nil {
		return fmt.Errorf("fromnode.r.Pathfind: %w", err)
	} else {
		pathfindSuccess = true
	}
	return nil
}
