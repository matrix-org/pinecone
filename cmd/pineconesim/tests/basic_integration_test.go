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
	"testing"
	"time"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator"
)

type TreeValidationState struct {
	roots       map[string]string
	correctRoot string
}

func TestNodesAgreeOnCorrectTreeRoot(t *testing.T) {
	t.Parallel()
	// Arrange
	scenario := NewScenarioFixture(t)
	nodes := []string{"Alice", "Bob"}
	scenario.AddStandardNodes(nodes)

	// Act
	scenario.AddPeerConnections([]NodePair{NodePair{"Alice", "Bob"}})

	// Assert
	stateCapture := func(state simulator.State) interface{} {
		lastRoots := make(map[string]string)
		lastRoots["Alice"] = state.Nodes["Alice"].Announcement.Root
		lastRoots["Bob"] = state.Nodes["Bob"].Announcement.Root

		correctRoot := ""
		if state.Nodes["Alice"].PeerID > state.Nodes["Bob"].PeerID {
			correctRoot = "Alice"
		} else {
			correctRoot = "Bob"
		}

		return TreeValidationState{roots: lastRoots, correctRoot: correctRoot}
	}

	nodesAgreeOnCorrectTreeRoot := func(prevState interface{}, event simulator.SimEvent) (interface{}, EventHandlerResult) {
		switch state := prevState.(type) {
		case TreeValidationState:
			action := DoNothing
			switch e := event.(type) {
			case simulator.TreeRootAnnUpdate:
				if state.roots[e.Node] != e.Root {
					log.Printf("Root changed for %s to %s", e.Node, e.Root)
					state.roots[e.Node] = e.Root
				} else {
					log.Printf("Got duplicate root info for %s", e.Node)
					break
				}

				if state.roots["Alice"] == state.roots["Bob"] {
					log.Println("Nodes agree on root")
					if state.roots["Alice"] == state.correctRoot {
						log.Println("The agreed root is the correct root")
						action = StartSettlingTimer
					} else {
						log.Println("The agreed root is not the correct root")
						action = StopSettlingTimer
					}
				} else {
					log.Println("Nodes disagree on root")
					action = StopSettlingTimer
				}
			}

			return state, action
		}

		return prevState, StopSettlingTimer
	}

	scenario.Validate(stateCapture, nodesAgreeOnCorrectTreeRoot, 2*time.Second, 5*time.Second)
}
