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
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"sort"
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

func main() {
	go func() {
		panic(http.ListenAndServe(":65432", nil))
	}()

	filename := flag.String("filename", "cmd/pineconesim/graphs/sim.txt", "the file that describes the simulated topology")
	sockets := flag.Bool("sockets", false, "use real TCP sockets to connect simulated nodes")
	chaos := flag.Int("chaos", 0, "randomly connect and disconnect a certain number of links")
	ping := flag.Bool("ping", false, "test end-to-end reachability between all nodes")
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
	sim := simulator.NewSimulator(log, *sockets, *ping)
	configureHTTPRouting(sim)
	sim.CalculateShortestPaths(nodes, wires)

	for n := range nodes {
		if err := sim.CreateNode(n); err != nil {
			panic(err)
		}
		sim.StartNodeEventHandler(n)
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

type pair struct{ from, to string }

const maxBatchSize int = 25

func userProxy(conn *websocket.Conn, sim *simulator.Simulator) {
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
		var peerConns []string
		for _, conn := range node.Connections {
			peerConns = append(peerConns, conn)
		}

		// Snake Links
		var snakeConns []string
		snakeConns = append(snakeConns, node.AscendingPeer)
		snakeConns = append(snakeConns, node.DescendingPeer)

		nodeState[name] = simulator.InitialNodeState{
			RootState: simulator.RootState{
				Root:        node.Announcement.Root,
				AnnSequence: node.Announcement.Sequence,
				AnnTime:     node.Announcement.Time,
				Coords:      node.Coords,
			},
			Peers:           peerConns,
			SnakeNeighbours: snakeConns,
			TreeParent:      node.Parent,
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

	// Start event handler for future sim events
	handleSimEvents(conn, ch)
}

func handleSimEvents(conn *websocket.Conn, ch chan simulator.SimEvent) {
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

		conn.WriteJSON(simulator.StateUpdateMsg{
			MsgID: simulator.SimUpdate,
			Event: simulator.SimEventMsg{
				UpdateID: eventType,
				Event:    event,
			}})
	}
}

func configureHTTPRouting(sim *simulator.Simulator) {
	http.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir("./cmd/pineconesim/ui"))))

	wsUpgrader := websocket.Upgrader{}
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
		c, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := util.WrapWebSocketConn(c)
		if _, err = n.AuthenticatedConnect(conn, "websocket", router.PeerTypeRemote, true); err != nil {
			return
		}
		log.Printf("WebSocket peer %q connected to sim node %q\n", c.RemoteAddr(), nodeID)
	})
	http.DefaultServeMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		var upgrader = websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		go userProxy(conn, sim)
	})
	http.DefaultServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles("./cmd/pineconesim/page.html"))
		nodes := sim.Nodes()

		totalCount := len(nodes) * len(nodes)
		dhtConvergence := 0
		pathConvergence := 0

		for _, nodes := range sim.TreePathConvergence() {
			for _, converged := range nodes {
				if converged {
					dhtConvergence++
				}
			}
		}

		for _, paths := range sim.SNEKPathConvergence() {
			for _, converged := range paths {
				if converged {
					pathConvergence++
				}
			}
		}

		data := PageData{
			TreeAvgStretch:      "Not tested",
			SNEKAvgStretch:      "Not tested",
			TreePathConvergence: "Not tested",
			SNEKPathConvergence: "Not tested",
			Uptime:              sim.Uptime().Round(time.Second),
		}
		if sim.PingingEnabled() && totalCount > 0 {
			data.TreePathConvergence = fmt.Sprintf("%d%%", (dhtConvergence*100)/totalCount)
			data.SNEKPathConvergence = fmt.Sprintf("%d%%", (pathConvergence*100)/totalCount)
		}
		shortcuts := map[string]string{}
		roots := map[string]int{}
		nodeids := []string{}
		for n := range nodes {
			nodeids = append(nodeids, n)
		}
		sort.Strings(nodeids)
		for _, n := range nodeids {
			node := nodes[n]
			public := node.PublicKey()
			asc, desc, table, stale := node.DHTInfo()
			entry := Node{
				Name:          n,
				Port:          "—",
				Coords:        fmt.Sprintf("%v", node.Coords()),
				Key:           hex.EncodeToString(public[:2]),
				IsRoot:        node.IsRoot(),
				DHTSize:       len(table),
				DHTStalePaths: stale,
			}
			if node.ListenAddr != nil {
				entry.Port = fmt.Sprintf("%d", node.ListenAddr.Port)
			}
			shortcuts[entry.Key] = n
			rootkey := node.RootPublicKey()
			entry.Root = hex.EncodeToString(rootkey[:2])
			if desc != nil {
				entry.Predecessor = hex.EncodeToString(desc.PublicKey[:2])
			}
			if asc != nil {
				entry.Successor = hex.EncodeToString(asc.PublicKey[:2])
			}
			data.Nodes = append(data.Nodes, entry)
			data.NodeCount++
			roots[entry.Root]++
		}

		if nodeKey := r.URL.Query().Get("pk"); nodeKey != "" {
			if node, ok := nodes[shortcuts[nodeKey]]; ok {
				pk := node.Router.PublicKey()
				asc, desc, dht, _ := node.Router.DHTInfo()
				pks := hex.EncodeToString(pk[:2])
				data.NodeInfo = &NodeInfo{
					Name:      shortcuts[pks],
					PublicKey: pks,
				}
				if asc != nil {
					data.NodeInfo.Ascending = hex.EncodeToString(asc.PublicKey[:2])
					data.NodeInfo.AscendingPathID = hex.EncodeToString(asc.PathID[:])
				}
				if desc != nil {
					data.NodeInfo.Descending = hex.EncodeToString(desc.PublicKey[:2])
					data.NodeInfo.DescendingPathID = hex.EncodeToString(desc.PathID[:])
				}
				for k, v := range dht {
					data.NodeInfo.Entries = append(
						data.NodeInfo.Entries,
						DHTEntry{
							PublicKey:       hex.EncodeToString(k.PublicKey[:2]),
							PathID:          hex.EncodeToString(k.PathID[:]),
							DestinationPort: v.Destination,
							SourcePort:      v.Source,
							Sequence:        int(v.Root.RootSequence),
						},
					)
				}
				for _, p := range node.Router.Peers() {
					data.NodeInfo.Peers = append(
						data.NodeInfo.Peers,
						PeerEntry{
							Name:          shortcuts[p.PublicKey[:4]],
							PublicKey:     p.PublicKey[:4],
							Port:          p.Port,
							RootPublicKey: p.RootPublicKey[:4],
						},
					)
				}
				sort.Sort(data.NodeInfo.Entries)
			}
		}

		data.PathCount = int(sim.State.GetLinkCount())

		for n, r := range roots {
			data.Roots = append(data.Roots, Root{
				Name:       n,
				References: r * 100 / len(data.Nodes),
			})
		}
		var stretchT float64
		var stretchS float64
		var stretchTC int
		var stretchSC int
		for a, aa := range sim.Distances() {
			for b, d := range aa {
				if a == b {
					continue
				}
				/*
					dist := Dist{
						From:         a,
						To:           b,
						Real:         "TBD",
						TreeObserved: "TBD",
						SNEKObserved: "TBD",
						TreeStretch:  "TBD",
						SNEKStretch:  "TBD",
					}
					dist.Real = fmt.Sprintf("%d", d.Real)
					if d.ObservedTree >= d.Real {
						dist.TreeObserved = fmt.Sprintf("%d", d.ObservedTree)
					}
					if d.ObservedSNEK >= d.Real {
						dist.SNEKObserved = fmt.Sprintf("%d", d.ObservedSNEK)
					}
				*/
				if d.ObservedTree >= d.Real {
					stretch := float64(1)
					if d.Real > 0 && d.ObservedTree > 0 {
						stretch = float64(1) / float64(d.Real) * float64(d.ObservedTree)
					}
					//dist.TreeStretch = fmt.Sprintf("%.2f", stretch)
					stretchT += stretch
					stretchTC++
				}
				if d.ObservedSNEK >= d.Real {
					stretch := float64(1)
					if d.Real > 0 && d.ObservedSNEK > 0 {
						stretch = float64(1) / float64(d.Real) * float64(d.ObservedSNEK)
					}
					//dist.SNEKStretch = fmt.Sprintf("%.2f", stretch)
					stretchS += stretch
					stretchSC++
				}
				// data.Dists = append(data.Dists, dist)
			}
		}
		if stretch := stretchT / float64(stretchTC); stretch >= 1 {
			data.TreeAvgStretch = fmt.Sprintf("%.2f", stretch)
		}
		if stretch := stretchS / float64(stretchSC); stretch >= 1 {
			data.SNEKAvgStretch = fmt.Sprintf("%.2f", stretch)
		}
		_ = tmpl.Execute(w, data)
	})
}

type Node struct {
	Name          string
	Port          string
	Coords        string
	Key           string
	Root          string
	Predecessor   string
	Successor     string
	IsRoot        bool
	IsExternal    bool
	DHTSize       int
	DHTStalePaths int
}

type Link struct {
	From    string
	To      string
	Enabled bool
}

type PageData struct {
	NodeCount           int
	PathCount           int
	Nodes               []Node
	Links               []Link
	Roots               []Root
	Dists               []Dist
	TreeAvgStretch      string
	SNEKAvgStretch      string
	TreePathConvergence string
	SNEKPathConvergence string
	NodeInfo            *NodeInfo
	Uptime              time.Duration
}

type NodeInfo struct {
	Name             string
	PublicKey        string
	Ascending        string
	AscendingPathID  string
	Descending       string
	DescendingPathID string
	Entries          DHTEntries
	Peers            PeerEntries
}

type DHTEntries []DHTEntry

func (e DHTEntries) Len() int {
	return len(e)
}
func (e DHTEntries) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}
func (e DHTEntries) Less(i, j int) bool {
	pk := strings.Compare(e[i].PublicKey, e[j].PublicKey)
	pi := strings.Compare(e[i].PathID, e[j].PathID)
	if pk == 0 {
		return pi < 0
	}
	return pk < 0
}

type DHTEntry struct {
	PublicKey       string
	DestinationPort interface{}
	SourcePort      interface{}
	PathID          string
	Sequence        int
}

type PeerEntries []PeerEntry

type PeerEntry struct {
	Name          string
	PublicKey     string
	Port          int
	RootPublicKey string
}

type Root struct {
	Name       string
	References int
}

type Dist struct {
	From         string
	To           string
	Real         string
	TreeObserved string
	SNEKObserved string
	TreeStretch  string
	SNEKStretch  string
}
