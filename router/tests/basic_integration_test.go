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
	"testing"
	"time"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator"
)

const SettlingTime time.Duration = time.Second * 2
const TestTimeout time.Duration = time.Second * 5

type TreeValidationState struct {
	roots       map[string]string
	correctRoot string
}

func TestNodesAgreeOnCorrectTreeRoot(t *testing.T) {
	t.Parallel()
	// Arrange
	scenario := NewScenarioFixture(t)
	nodes := []string{"Alice", "Bob", "Charlie"}
	scenario.AddStandardNodes(nodes)

	// Act
	scenario.AddPeerConnections([]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}})

	// Assert
	stateCapture := func(state simulator.State) interface{} {
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

	scenario.Validate(stateCapture, nodesAgreeOnCorrectTreeRoot, SettlingTime, TestTimeout)
}

type SnakeNeighbours struct {
	asc  string
	desc string
}

type SnakeValidationState struct {
	snake        map[string]SnakeNeighbours
	correctSnake map[string]SnakeNeighbours
}

type Node struct {
	name string
	key  string
}

type byKey []Node

func (l byKey) Len() int {
	return len(l)
}

func (l byKey) Less(i, j int) bool {
	return l[i].key < l[j].key
}

func (l byKey) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func TestNodesAgreeOnCorrectSnakeFormation(t *testing.T) {
	t.Parallel()
	// Arrange
	scenario := NewScenarioFixture(t)
	nodes := []string{"Alice", "Bob", "Charlie"}
	scenario.AddStandardNodes(nodes)

	// Act
	scenario.AddPeerConnections([]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}})

	// Assert
	stateCapture := func(state simulator.State) interface{} {
		snakeNeighbours := make(map[string]SnakeNeighbours)
		for _, node := range nodes {
			asc := state.Nodes[node].AscendingPeer
			desc := state.Nodes[node].DescendingPeer
			snakeNeighbours[node] = SnakeNeighbours{asc: asc, desc: desc}
		}

		nodesByKey := make(byKey, 0, len(state.Nodes))
		for key, value := range state.Nodes {
			nodesByKey = append(nodesByKey, Node{key, value.PeerID})
		}
		sort.Sort(nodesByKey)

		correctSnake := make(map[string]SnakeNeighbours)
		lowest := SnakeNeighbours{asc: nodesByKey[1].name, desc: ""}
		middle := SnakeNeighbours{asc: nodesByKey[2].name, desc: nodesByKey[0].name}
		highest := SnakeNeighbours{asc: "", desc: nodesByKey[1].name}
		correctSnake[nodesByKey[0].name] = lowest
		correctSnake[nodesByKey[1].name] = middle
		correctSnake[nodesByKey[2].name] = highest

		return SnakeValidationState{snakeNeighbours, correctSnake}
	}

	nodesAgreeOnCorrectSnakeFormation := func(prevState interface{}, event simulator.SimEvent) (interface{}, EventHandlerResult) {
		switch state := prevState.(type) {
		case SnakeValidationState:
			isSnakeCorrect := func() bool {
				snakeIsCorrect := true
				for key, val := range state.snake {
					if val.asc != state.correctSnake[key].asc || val.desc != state.correctSnake[key].desc {
						snakeIsCorrect = false
						break
					}
				}
				return snakeIsCorrect
			}

			snakeWasCorrect := isSnakeCorrect()

			action := DoNothing
			updateReceived := false
			switch e := event.(type) {
			case simulator.SnakeAscUpdate:
				updateReceived = true
				if node, ok := state.snake[e.Node]; ok {
					node.asc = e.Peer
					state.snake[e.Node] = node
				}
			case simulator.SnakeDescUpdate:
				updateReceived = true
				if node, ok := state.snake[e.Node]; ok {
					node.desc = e.Peer
					state.snake[e.Node] = node
				}
			}

			if updateReceived {
				if isSnakeCorrect() && !snakeWasCorrect {
					action = StartSettlingTimer
				} else {
					action = StopSettlingTimer
				}
			}

			return state, action
		}

		return prevState, StopSettlingTimer
	}

	scenario.Validate(stateCapture, nodesAgreeOnCorrectSnakeFormation, SettlingTime, TestTimeout)
}
