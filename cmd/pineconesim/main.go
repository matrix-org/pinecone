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

package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator"
	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/util"
	"go.uber.org/atomic"

	"net/http"
	_ "net/http/pprof"
)

type pair struct{ from, to string }

const maxBatchSize int = 50

var ConnUID atomic.Uint64 = atomic.Uint64{}

func main() {
	go func() {
		panic(http.ListenAndServe(":65432", nil))
	}()

	filename := flag.String("filename", "cmd/pineconesim/graphs/empty.txt", "the file that describes the simulated topology")
	sockets := flag.Bool("sockets", false, "use real TCP sockets to connect simulated nodes")
	chaos := flag.Int("chaos", 0, "randomly connect and disconnect a certain number of links")
	ping := flag.Bool("ping", false, "test end-to-end reachability between all nodes")
	acceptCommands := flag.Bool("acceptCommands", true, "whether the sim can be commanded from the ui")
	flag.Parse()

	file, err := os.Open(*filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	nodes := map[string]struct{}{}
	wires := map[string]map[string]bool{}

	for scanner.Scan() {
		tokens := strings.Split(strings.TrimSpace(scanner.Text()), " ")
		for _, t := range tokens {
			nodes[t] = struct{}{}
		}
		for i := 1; i < len(tokens); i++ {
			a, b := tokens[i-1], tokens[i]
			if _, ok := wires[a]; !ok {
				wires[a] = map[string]bool{}
			}
			if _, ok := wires[b][a]; ok {
				continue
			}
			wires[a][b] = false
		}
	}

	log := log.New(os.Stdout, "\u001b[36m***\u001b[0m ", 0)
	sim := simulator.NewSimulator(log, *sockets, *ping, *acceptCommands)
	configureHTTPRouting(log, sim)
	sim.CalculateShortestPaths(nodes, wires)

	for n := range nodes {
		if err := sim.CreateNode(n, simulator.DefaultNode); err != nil {
			panic(err)
		}
		sim.StartNodeEventHandler(n, simulator.DefaultNode)
	}

	for a, w := range wires {
		for b := range w {
			if a == b {
				continue
			}
			log.Printf("Connecting %q and %q...\n", a, b)
			err := sim.ConnectNodes(a, b)
			wires[a][b] = err == nil
			if err != nil {
				continue
			}
		}
	}

	if chaos != nil && *chaos > 0 {
		rand.Seed(time.Now().UnixNano())
		maxintv, maxswing := 20, int32(*chaos)
		var swing atomic.Int32

		// Chaos disconnector
		go func() {
			for {
				if swing.Load() > -maxswing {
				parentloop:
					for a, w := range wires {
						for b, s := range w {
							if !s {
								continue
							}
							if err := sim.DisconnectNodes(a, b); err == nil {
								wires[a][b] = false
								swing.Dec()
								break parentloop
							}
						}
					}
				}
				time.Sleep(time.Second * time.Duration(rand.Intn(maxintv)))
			}
		}()

		// Chaos connector
		go func() {
			for {
				if swing.Load() < maxswing {
				parentloop:
					for a, w := range wires {
						for b, s := range w {
							if s {
								continue
							}
							if err := sim.ConnectNodes(a, b); err == nil {
								wires[a][b] = true
								swing.Inc()
								break parentloop
							}
						}
					}
				}
				time.Sleep(time.Second * time.Duration(rand.Intn(maxintv)))
			}
		}()
	}

	log.Println("Configuring HTTP listener")

	if ping != nil && *ping {
		go func() {
			for {
				time.Sleep(time.Second * 15)
				log.Println("Starting pings...")

				tasks := make(chan pair, 2*(len(nodes)*len(nodes)))
				for from := range nodes {
					for to := range nodes {
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
							log.Println("Tree ping from", pair.from, "to", pair.to)
							if _, _, err := sim.PingTree(pair.from, pair.to); err != nil {
								log.Println("Tree ping from", pair.from, "to", pair.to, "failed:", err)
							}
							log.Println("SNEK ping from", pair.from, "to", pair.to)
							if _, _, err := sim.PingSNEK(pair.from, pair.to); err != nil {
								log.Println("SNEK ping from", pair.from, "to", pair.to, "failed:", err)
							}
						}
						wg.Done()
					}()
				}

				wg.Wait()
				log.Println("All pings finished, repeating shortly...")
			}
		}()
	}

	select {}
}

func configureHTTPRouting(log *log.Logger, sim *simulator.Simulator) {
	var upgrader = websocket.Upgrader{}
	http.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir("./cmd/pineconesim/ui"))))

	http.DefaultServeMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		log.Println("New websocket connection established")

		connID := ConnUID.Inc()
		go userProxyReporter(conn, connID, sim)
		if sim.AcceptCommands {
			go userProxyCommander(conn, connID, sim)
		}
	})

	http.DefaultServeMux.HandleFunc("/simws", func(w http.ResponseWriter, r *http.Request) {
		var n *simulator.Node
		nodeID := r.URL.Query().Get("node")
		if nodeID != "" {
			n = sim.Node(nodeID)
		} else {
			for id, node := range sim.Nodes() {
				if node != nil {
					n, nodeID = node, id
					break
				}
			}
		}
		if n == nil {
			w.WriteHeader(404)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := util.WrapWebSocketConn(c)
		if _, err = n.Connect(
			conn,
			router.ConnectionZone("websocket"),
			router.ConnectionPeerType(router.PeerTypeRemote),
		); err != nil {
			return
		}
		log.Printf("WebSocket peer %q connected to sim node %q\n", c.RemoteAddr(), nodeID)
	})

	http.DefaultServeMux.HandleFunc("/manhole", func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.URL.Query().Get("node")
		node := sim.Node(nodeID)
		if node == nil {
			w.WriteHeader(404)
			return
		}
		node.SimRouter.ManholeHandler(w, r)
	})

	http.DefaultServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_ = template.Must(template.ParseFiles("./cmd/pineconesim/page.html")).Execute(w, "")
	})
}

func userProxyReporter(conn *websocket.Conn, connID uint64, sim *simulator.Simulator) {
	log := log.New(os.Stdout, fmt.Sprintf("\u001b[38;5;93mWsSimReporter::%d ***\u001b[0m ", connID), 0)
	log.Println("Listening to sim for updates...")

	// Subscribe to sim events and grab snapshot of current state
	ch := make(chan simulator.SimEvent)
	state := sim.State.Subscribe(ch)

	// Split state into batches and send to the UI
	end := false
	batchSize := 0
	totalNodesProcessed := 0
	nodeState := make(map[string]simulator.InitialNodeState, maxBatchSize)
	for name, node := range state.Nodes {
		batchSize++
		totalNodesProcessed++
		if totalNodesProcessed == len(state.Nodes) {
			end = true
		}

		// Peer Links
		var peerConns []simulator.PeerInfo
		for port, conn := range node.Connections {
			peerConns = append(peerConns, simulator.PeerInfo{ID: conn, Port: port})
		}

		nodeState[name] = simulator.InitialNodeState{
			PublicKey: node.PeerID,
			NodeType:  node.NodeType,
			RootState: simulator.RootState{
				Root:        node.Announcement.Root,
				AnnSequence: node.Announcement.Sequence,
				AnnTime:     node.Announcement.Time,
				Coords:      node.Coords,
			},
			Peers:         peerConns,
			TreeParent:    node.Parent,
			SnakeAsc:      node.AscendingPeer,
			SnakeAscPath:  node.AscendingPathID,
			SnakeDesc:     node.DescendingPeer,
			SnakeDescPath: node.DescendingPathID,
		}

		if batchSize == int(maxBatchSize) || end {
			// Send batch
			if err := conn.WriteJSON(simulator.InitialStateMsg{
				MsgID: simulator.SimInitialState,
				Nodes: nodeState,
				End:   end,
			}); err != nil {
				log.Println(err)
				return
			}

			// Reset Batch Info
			batchSize = 0
			nodeState = make(map[string]simulator.InitialNodeState, maxBatchSize)
		}
	}

	// In the case the sim starts with an empty graph, send an empty initial state message
	// to let the UI know it can begin processing updates.
	if len(state.Nodes) == 0 {
		if err := conn.WriteJSON(simulator.InitialStateMsg{
			MsgID: simulator.SimInitialState,
			Nodes: map[string]simulator.InitialNodeState{},
			End:   true,
		}); err != nil {
			log.Println(err)
			return
		}
	}

	// Start event handler for future sim events
	handleSimEvents(log, conn, ch)
	log.Printf("Closing WsSimReporter::%d\n", connID)
}

func handleSimEvents(log *log.Logger, conn *websocket.Conn, ch <-chan simulator.SimEvent) {
	for {
		event := <-ch
		eventType := simulator.UnknownUpdate
		switch event.(type) {
		case simulator.NodeAdded:
			eventType = simulator.SimNodeAdded
		case simulator.NodeRemoved:
			eventType = simulator.SimNodeRemoved
		case simulator.PeerAdded:
			eventType = simulator.SimPeerAdded
		case simulator.PeerRemoved:
			eventType = simulator.SimPeerRemoved
		case simulator.TreeParentUpdate:
			eventType = simulator.SimTreeParentUpdated
		case simulator.SnakeAscUpdate:
			eventType = simulator.SimSnakeAscUpdated
		case simulator.SnakeDescUpdate:
			eventType = simulator.SimSnakeDescUpdated
		case simulator.TreeRootAnnUpdate:
			eventType = simulator.SimTreeRootAnnUpdated
		}

		if err := conn.WriteJSON(simulator.StateUpdateMsg{
			MsgID: simulator.SimStateUpdate,
			Event: simulator.SimEventMsg{
				UpdateID: eventType,
				Event:    event,
			}}); err != nil {
			log.Println(err)
			return
		}
	}
}

func userProxyCommander(conn *websocket.Conn, connID uint64, sim *simulator.Simulator) {
	log := log.New(os.Stdout, fmt.Sprintf("\u001b[38;5;220mWsSimCommander::%d ***\u001b[0m ", connID), 0)
	log.Println("Listening to ui for commands...")

	for {
		var eventSequence simulator.SimCommandSequenceMsg
		if err := conn.ReadJSON(&eventSequence); err != nil {
			log.Println(err)
			if strings.Contains(err.Error(), "unmarshal") {
				continue
			} else {
				log.Printf("Unhandled websocket failure, closing WsSimCommander::%d\n", connID)
				return
			}
		}

		// Unmarshall the events into meaningful structs
		var commands []simulator.SimCommand
		for i, event := range eventSequence.Events {
			command, err := simulator.UnmarshalCommandJSON(&event)

			if err != nil {
				escapedErr := strings.Replace(err.Error(), "\n", "", -1)
				escapedErr = strings.Replace(escapedErr, "\r", "", -1)
				log.Printf("Index %d: %v", i, escapedErr)
				continue
			}

			commands = append(commands, command)
		}

		if len(commands) > 0 {
			k := 0
			keepCommand := func(i int, cmd simulator.SimCommand) {
				if i != k {
					commands[k] = cmd
				}
				k++
			}
			for i, cmd := range commands {
				switch cmd.(type) {
				case simulator.Play:
					if len(commands) > 1 {
						log.Println("Play removed from sequence")
					} else {
						cmd.Run(log, sim)
					}
				case simulator.Pause:
					if len(commands) > 1 {
						log.Println("Pause removed from sequence")
					} else {
						cmd.Run(log, sim)
					}
				default:
					keepCommand(i, cmd)
				}
			}

			commands = commands[:k]

			if len(commands) > 0 {
				log.Printf("Adding the following commands to be played: %v", commands)
				sim.AddToPlaylist(commands)
			}
		}
	}
}
