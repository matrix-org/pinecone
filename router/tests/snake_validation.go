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
	"sync"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator"
)

type SnakeNeighbours struct {
	asc  string
	desc string
}

type SnakeValidationState struct {
	snake        map[string]SnakeNeighbours
	correctSnake map[string]SnakeNeighbours
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func createSnakeStateCapture(nodes []string) InitialStateCapture {
	return func(state simulator.State) interface{} {
		snakeNeighbours := make(map[string]SnakeNeighbours)
		for _, node := range nodes {
			asc := state.Nodes[node].AscendingPeer
			desc := state.Nodes[node].DescendingPeer
			snakeNeighbours[node] = SnakeNeighbours{asc: asc, desc: desc}
		}

		nodesByKey := make(byKey, 0, len(state.Nodes))
		for key, value := range state.Nodes {
			if contains(nodes, key) {
				nodesByKey = append(nodesByKey, Node{key, value.PeerID})
			}
		}
		sort.Sort(nodesByKey)

		correctSnake := make(map[string]SnakeNeighbours)
		for i, node := range nodesByKey {
			asc := ""
			desc := ""
			if i == 0 {
				if len(nodesByKey) > 1 {
					asc = nodesByKey[i+1].name
				}
			} else if i == len(nodesByKey)-1 {
				if len(nodesByKey) > 1 {
					desc = nodesByKey[i-1].name
				}
			} else {
				asc = nodesByKey[i+1].name
				desc = nodesByKey[i-1].name
			}

			correctSnake[node.name] = SnakeNeighbours{asc: asc, desc: desc}
		}

		return SnakeValidationState{snakeNeighbours, correctSnake}
	}
}

func nodesAgreeOnCorrectSnakeFormation(prevState interface{}, event simulator.SimEvent, isSettling bool) (newState interface{}, result EventHandlerResult) {
	switch state := prevState.(type) {
	case SnakeValidationState:
		snakeWasCorrect := isSettling
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

		switch e := event.(type) {
		case simulator.SnakeAscUpdate:
			if node, ok := state.snake[e.Node]; ok {
				node.asc = e.Peer
				state.snake[e.Node] = node
			}
		case simulator.SnakeDescUpdate:
			if node, ok := state.snake[e.Node]; ok {
				node.desc = e.Peer
				state.snake[e.Node] = node
			}
		}

		action := DoNothing
		isCorrect := isSnakeCorrect()
		if isCorrect && !snakeWasCorrect {
			action = StartSettlingTimer
		} else if !isCorrect {
			action = StopSettlingTimer
		}

		return state, action
	}

	return prevState, StopSettlingTimer
}

func pingNode(scenario *ScenarioFixture, from string, to string, ch chan bool, wg *sync.WaitGroup) {
	log.Printf("Pinging from %s to %s", from, to)
	passed := true
	if _, _, err := scenario.sim.PingSNEK(from, to); err != nil {
		passed = false
		scenario.t.Errorf("Failed pinging from %s to %s: %s", from, to, err)
	}

	ch <- passed
	wg.Done()
}

func nodesCanAllPingEachOther(scenario *ScenarioFixture, nodes []string) bool {
	wg := sync.WaitGroup{}

	numberOfPings := 0
	for range nodes {
		for range nodes {
			numberOfPings++
		}
	}

	ch := make(chan bool, numberOfPings)

	for _, from := range nodes {
		for _, to := range nodes {
			if from != to {
				wg.Add(1)
				go pingNode(scenario, from, to, ch, &wg)
			}
		}
	}

	wg.Wait()
	close(ch)

	overallPass := true
	for passed := range ch {
		if !passed {
			overallPass = false
		}
	}

	return overallPass
}
