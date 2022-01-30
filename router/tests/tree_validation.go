// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package integration

import (
	"log"
	"sort"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator"
)

type TreeValidationState struct {
	roots       map[string]string
	correctRoot string
}

func createTreeStateCapture(nodes []string) InitialStateCapture {
	return func(state simulator.State) interface{} {
		lastRoots := make(map[string]string)
		for _, node := range nodes {
			lastRoots[node] = state.Nodes[node].Announcement.Root
		}

		nodesByKey := make(byKey, 0, len(state.Nodes))
		for key, value := range state.Nodes {
			nodesByKey = append(nodesByKey, Node{key, value.PeerID})
		}
		sort.Sort(nodesByKey)

		correctRoot := nodesByKey[len(nodesByKey)-1].name

		return TreeValidationState{roots: lastRoots, correctRoot: correctRoot}
	}
}

func nodesAgreeOnCorrectTreeRoot(prevState interface{}, event simulator.SimEvent) (newState interface{}, result EventHandlerResult) {
	switch state := prevState.(type) {
	case TreeValidationState:
		action := DoNothing
		switch e := event.(type) {
		case simulator.TreeRootAnnUpdate:
			if _, ok := state.roots[e.Node]; !ok {
				// NOTE : only process events for nodes we care about
				break
			}

			if state.roots[e.Node] != e.Root {
				log.Printf("Root changed for %s to %s", e.Node, e.Root)
				state.roots[e.Node] = e.Root
			} else {
				log.Printf("Got duplicate root info for %s", e.Node)
				break
			}

			nodesAgreeOnRoot := true
			rootSample := ""
			for _, node := range state.roots {
				rootSample = node
				for _, comparison := range state.roots {
					if node != comparison {
						nodesAgreeOnRoot = false
						break
					}
				}
			}

			if nodesAgreeOnRoot && state.correctRoot == rootSample {
				log.Println("Start settling for tree test")
				action = StartSettlingTimer
			} else {
				log.Println("Stop settling for tree test")
				action = StopSettlingTimer
			}
		}

		return state, action
	}

	return prevState, StopSettlingTimer
}
