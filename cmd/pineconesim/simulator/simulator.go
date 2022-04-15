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
	"crypto/ed25519"
	"log"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/Arceliar/phony"
	"github.com/RyanCarrier/dijkstra"
	"go.uber.org/atomic"
)

type EventSequenceRunner struct {
	phony.Inbox
	_playlist  chan []SimCommand
	_isPlaying atomic.Bool
}

func (r *EventSequenceRunner) Play() {
	r._isPlaying.Store(true)
}

func (r *EventSequenceRunner) Pause() {
	r._isPlaying.Store(false)
}

func (r *EventSequenceRunner) Run(sim *Simulator) {
	for commands := range r._playlist {
		sim.log.Printf("Executing new command sequence: %v", commands)
		for _, cmd := range commands {
			// Only process commands while in play mode
			for {
				if r._isPlaying.Load() {
					break
				}

				// TODO : make this less sleepy with another channel for admin
				time.Sleep(time.Duration(200) * time.Millisecond)
			}

			cmd.Run(sim.log, sim)
		}
		sim.log.Println("Finished executing command sequence")
	}
}

type RouterCreatorFn func(log *log.Logger, sk ed25519.PrivateKey, debug bool) SimRouter

type pair struct{ from, to string }

type Simulator struct {
	log                      *log.Logger
	sockets                  bool
	ping                     bool
	AcceptCommands           bool
	nodes                    map[string]*Node
	nodesMutex               sync.RWMutex
	nodeRunnerChannels       map[string][]chan<- bool
	nodeRunnerChannelsMutex  sync.RWMutex
	graph                    *dijkstra.Graph
	maps                     map[string]int
	wires                    map[string]map[string]net.Conn
	wiresMutex               sync.RWMutex
	dists                    map[string]map[string]*Distance
	distsMutex               sync.RWMutex
	snekPathConvergence      map[string]map[string]bool
	snekPathConvergenceMutex sync.RWMutex
	treePathConvergence      map[string]map[string]bool
	treePathConvergenceMutex sync.RWMutex
	startTime                time.Time
	State                    *StateAccessor
	eventPlaylist            []SimCommand
	eventRunner              *EventSequenceRunner
	routerCreationMap        map[APINodeType]RouterCreatorFn
	pingControlChannel       chan<- bool
}

func NewSimulator(log *log.Logger, sockets, acceptCommands bool) *Simulator {
	sim := &Simulator{
		log:                 log,
		sockets:             sockets,
		ping:                false,
		AcceptCommands:      acceptCommands,
		nodes:               make(map[string]*Node),
		nodeRunnerChannels:  make(map[string][]chan<- bool),
		wires:               make(map[string]map[string]net.Conn),
		dists:               make(map[string]map[string]*Distance),
		snekPathConvergence: make(map[string]map[string]bool),
		treePathConvergence: make(map[string]map[string]bool),
		startTime:           time.Now(),
		State:               NewStateAccessor(),
		eventPlaylist:       []SimCommand{},
		eventRunner:         &EventSequenceRunner{_playlist: make(chan []SimCommand)},
		routerCreationMap:   make(map[APINodeType]RouterCreatorFn, 2),
		pingControlChannel:  make(chan<- bool),
	}

	sim.routerCreationMap[DefaultNode] = createDefaultRouter
	sim.routerCreationMap[GeneralAdversaryNode] = createAdversaryRouter

	go sim.eventRunner.Run(sim)
	sim.Play()

	return sim
}

func (sim *Simulator) StartPinging(ping_period time.Duration) {
	quit := make(chan bool)
	go func(quit <-chan bool) {
		defer sim.resetPingState()
		for {
			select {
			case <-quit:
				sim.log.Println("Stopping pings.")
				return
			default:
				// TODO : Send ui pings running
				sim.log.Println("Starting pings...")

				tasks := make(chan pair, 2*(len(sim.nodes)*len(sim.nodes)))
				for from := range sim.nodes {
					for to := range sim.nodes {
						tasks <- pair{from, to}
					}
				}
				close(tasks)

				numworkers := runtime.NumCPU() * 16
				var wg sync.WaitGroup
				wg.Add(numworkers)
				for i := 0; i < numworkers; i++ {
					go func() {
						for pair := range tasks {
							sim.log.Println("Tree ping from", pair.from, "to", pair.to)
							if _, _, err := sim.PingTree(pair.from, pair.to); err != nil {
								sim.log.Println("Tree ping from", pair.from, "to", pair.to, "failed:", err)
							}
							sim.log.Println("SNEK ping from", pair.from, "to", pair.to)
							if _, _, err := sim.PingSNEK(pair.from, pair.to); err != nil {
								sim.log.Println("SNEK ping from", pair.from, "to", pair.to, "failed:", err)
							}
						}
						wg.Done()
					}()
				}

				wg.Wait()

				select {
				case <-quit:
					sim.log.Println("Stopping pings.")
					return
				default:
					// TODO : Send ui pings waiting
					sim.log.Println("All pings finished, repeating shortly...")
					time.Sleep(ping_period)
				}
			}
		}
	}(quit)

	sim.pingControlChannel = quit
}

func (sim *Simulator) PingingEnabled() bool {
	return sim.ping
}

func (sim *Simulator) Nodes() map[string]*Node {
	sim.nodesMutex.RLock()
	defer sim.nodesMutex.RUnlock()
	return sim.nodes
}

func (sim *Simulator) Distances() map[string]map[string]*Distance {
	sim.distsMutex.RLock()
	defer sim.distsMutex.RUnlock()
	mapcopy := make(map[string]map[string]*Distance)
	for a, aa := range sim.dists {
		if _, ok := mapcopy[a]; !ok {
			mapcopy[a] = make(map[string]*Distance)
		}
		for b, bb := range aa {
			mapcopy[a][b] = bb
		}
	}
	return mapcopy
}

func (sim *Simulator) SNEKPathConvergence() map[string]map[string]bool {
	sim.snekPathConvergenceMutex.RLock()
	defer sim.snekPathConvergenceMutex.RUnlock()
	mapcopy := make(map[string]map[string]bool)
	for a, aa := range sim.snekPathConvergence {
		if _, ok := mapcopy[a]; !ok {
			mapcopy[a] = make(map[string]bool)
		}
		for b, bb := range aa {
			mapcopy[a][b] = bb
		}
	}
	return mapcopy
}

func (sim *Simulator) TreePathConvergence() map[string]map[string]bool {
	sim.treePathConvergenceMutex.RLock()
	defer sim.treePathConvergenceMutex.RUnlock()
	mapcopy := make(map[string]map[string]bool)
	for a, aa := range sim.treePathConvergence {
		if _, ok := mapcopy[a]; !ok {
			mapcopy[a] = make(map[string]bool)
		}
		for b, bb := range aa {
			mapcopy[a][b] = bb
		}
	}
	return mapcopy
}

func (sim *Simulator) Uptime() time.Duration {
	return time.Since(sim.startTime)
}

func (sim *Simulator) handlePeerAdded(node string, peerID string, port int) {
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		sim.State.Act(nil, func() { sim.State._addPeerConnection(node, peerNode, port) })
	}
}

func (sim *Simulator) handlePeerRemoved(node string, peerID string, port int) {
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		sim.DisconnectNodes(node, peerNode)
		sim.State.Act(nil, func() { sim.State._removePeerConnection(node, peerNode, port) })
	}
}

func (sim *Simulator) handleTreeParentUpdate(node string, peerID string) {
	peerName := ""
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		peerName = peerNode
	}
	sim.State.Act(nil, func() { sim.State._updateParent(node, peerName) })
}

func (sim *Simulator) handleSnakeAscUpdate(node string, peerID string, pathID string) {
	peerName := ""
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		peerName = peerNode
	}

	sim.State.Act(nil, func() { sim.State._updateAscendingPeer(node, peerName, pathID) })
}

func (sim *Simulator) handleSnakeDescUpdate(node string, peerID string, pathID string) {
	peerName := ""
	if peerNode, err := sim.State.GetNodeName(peerID); err == nil {
		peerName = peerNode
	}
	sim.State.Act(nil, func() { sim.State._updateDescendingPeer(node, peerName, pathID) })
}

func (sim *Simulator) handleTreeRootAnnUpdate(node string, root string, sequence uint64, time uint64, coords []uint64) {
	rootName := ""
	if peerNode, err := sim.State.GetNodeName(root); err == nil {
		rootName = peerNode
	}
	sim.State.Act(nil, func() { sim.State._updateTreeRootAnnouncement(node, rootName, sequence, time, coords) })
}

func (sim *Simulator) updatePingState(active bool) {
	sim.State.Act(nil, func() { sim.State._publish(PingStateUpdate{Active: active}) })
}

type EventSequencePlayer interface {
	Play()
	Pause()
	AddToPlaylist(commands []SimCommand)
}

func (sim *Simulator) Play() {
	sim.eventRunner.Play()
}

func (sim *Simulator) Pause() {
	sim.eventRunner.Pause()
}

func (sim *Simulator) StartPings() {
	if !sim.ping {
		sim.updatePingState(true)
		sim.ping = true
		sim.StartPinging(time.Second * 15)
	}
}

func (sim *Simulator) resetPingState() {
	sim.updatePingState(false)
	sim.ping = false
}

func (sim *Simulator) StopPings() {
	if sim.ping {
		go func() {
			sim.pingControlChannel <- true
		}()
	}
}

func (sim *Simulator) AddToPlaylist(commands []SimCommand) {
	// NOTE : Pass a list of commands instead of individual commands to enforce
	// command sets being run without interleaving events from other command sets
	sim.eventRunner.Act(nil, func() {
		sim.eventRunner._playlist <- commands
	})
}
