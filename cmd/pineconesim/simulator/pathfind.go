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
)

func (sim *Simulator) Ping(from, to string) (uint16, time.Duration, error) {
	fromnode := sim.nodes[from]
	tonode := sim.nodes[to]
	success := false

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	defer func() {
		sim.pathConvergenceMutex.Lock()
		if _, ok := sim.pathConvergence[from]; !ok {
			sim.pathConvergence[from] = map[string]bool{}
		}
		sim.pathConvergence[from][to] = success
		sim.pathConvergenceMutex.Unlock()
	}()

	hops, rtt, err := fromnode.Ping(ctx, tonode.PublicKey())
	if err != nil {
		return 0, 0, fmt.Errorf("fromnode.Ping: %w", err)
	}

	success = true
	sim.ReportDistance(from, to, int64(hops))
	return hops, rtt, nil
}
