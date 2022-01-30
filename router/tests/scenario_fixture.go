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
	"crypto/ed25519"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator"
	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator/adversary"
)

type EventHandlerResult int

const (
	DoNothing EventHandlerResult = iota
	StopSettlingTimer
	StartSettlingTimer
)

type InitialStateCapture func(state simulator.State) (initialState interface{})
type EventHandler func(prevState interface{}, event simulator.SimEvent) (newState interface{}, result EventHandlerResult)

type NodePair struct {
	A string
	B string
}

type ScenarioFixture struct {
	t   *testing.T
	log *log.Logger
	sim *simulator.Simulator
}

func NewScenarioFixture(t *testing.T) ScenarioFixture {
	log := log.New(os.Stdout, "\u001b[36m***\u001b[0m ", 0)
	useSockets := false
	runPing := false
	acceptCommands := true
	simulator := simulator.NewSimulator(log, useSockets, runPing, acceptCommands)

	return ScenarioFixture{
		t:   t,
		log: log,
		sim: simulator,
	}
}

func (s *ScenarioFixture) AddStandardNodes(nodes []string) {
	for _, node := range nodes {
		cmd := simulator.AddNode{
			Node:     node,
			NodeType: simulator.DefaultNode,
		}
		cmd.Run(s.log, s.sim)
	}
}

func (s *ScenarioFixture) AddStandardNodesWithKeys(nodes map[string]*ed25519.PrivateKey) {
	for node, key := range nodes {
		cmd := simulator.AddNode{
			Node:     node,
			NodeType: simulator.DefaultNode,
			PrivKey:  key,
		}
		cmd.Run(s.log, s.sim)
	}
}

func (s *ScenarioFixture) AddAdversaryNodes(nodes []string) {
	for _, node := range nodes {
		cmd := simulator.AddNode{
			Node:     node,
			NodeType: simulator.GeneralAdversaryNode,
		}
		cmd.Run(s.log, s.sim)
	}
}

func (s *ScenarioFixture) AddAdversaryNodesWithKeys(nodes map[string]*ed25519.PrivateKey) {
	for node, key := range nodes {
		cmd := simulator.AddNode{
			Node:     node,
			NodeType: simulator.GeneralAdversaryNode,
			PrivKey:  key,
		}
		cmd.Run(s.log, s.sim)
	}
}

func (s *ScenarioFixture) AddPeerConnections(conns []NodePair) {
	for _, pair := range conns {
		cmd := simulator.AddPeer{
			Node: pair.A,
			Peer: pair.B,
		}
		cmd.Run(s.log, s.sim)
	}
}

func (s *ScenarioFixture) ConfigureAdversary(node string, defaults adversary.DropRates, peerDropRates map[string]adversary.DropRates) {
	s.sim.ConfigureFilterDefaults(node, defaults)
	for peer, rates := range peerDropRates {
		s.sim.ConfigureFilterPeer(node, peer, rates)
	}
}

func (s *ScenarioFixture) SubscribeToSimState(ch chan simulator.SimEvent) simulator.State {
	return s.sim.State.Subscribe(ch)
}

func (s *ScenarioFixture) Validate(initialState InitialStateCapture, eventHandler EventHandler, settlingTime time.Duration, timeout time.Duration) {
	testTimeout := time.NewTimer(timeout)
	defer testTimeout.Stop()

	quit := make(chan bool)
	output := make(chan string)
	go assertState(s, initialState, eventHandler, quit, output, settlingTime)

	failed := false

	select {
	case <-testTimeout.C:
		failed = true
		quit <- true
	case <-output:
		log.Println("Test passed")
	}

	if failed {
		state := <-output
		s.t.Fatalf("Test timeout reached. Current State: %s", state)
	}
}

func assertState(scenario *ScenarioFixture, stateCapture InitialStateCapture, eventHandler EventHandler, quit chan bool, output chan string, settlingTime time.Duration) {
	settlingTimer := time.NewTimer(settlingTime)
	settlingTimer.Stop()

	simUpdates := make(chan simulator.SimEvent)
	state := scenario.SubscribeToSimState(simUpdates)

	prevState := stateCapture(state)

	for {
		select {
		case <-quit:
			output <- fmt.Sprintf("%+v", prevState)
			return
		case <-settlingTimer.C:
			output <- "PASS"
		case event := <-simUpdates:
			newState, newResult := eventHandler(prevState, event)
			switch newResult {
			case StartSettlingTimer:
				log.Println("Starting settling timer")
				settlingTimer.Reset(settlingTime)
			case StopSettlingTimer:
				log.Println("Stopping settling timer")
				settlingTimer.Stop()
			}

			prevState = newState
		}
	}
}
