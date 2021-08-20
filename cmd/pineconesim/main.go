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

	"net/http"
	_ "net/http/pprof"
)

func main() {
	go func() {
		panic(http.ListenAndServe(":65432", nil))
	}()

	filename := flag.String("filename", "cmd/pineconesim/graphs/sim.txt", "the file that describes the simulated topology")
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
	sim := simulator.NewSimulator(log)
	configureHTTPRouting(sim)
	sim.CalculateShortestPaths(nodes, wires)

	for n := range nodes {
		if err := sim.CreateNode(n); err != nil {
			panic(err)
		}
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

	/*
		rand.Seed(time.Now().UnixNano())
		maxintv, maxswing := 5, int32(10)
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
	*/

	log.Println("Configuring HTTP listener")

	go func() {
		for {
			time.Sleep(time.Second * 15)
			log.Println("Starting pathfinds...")

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
						log.Println("Tree pathfind from", pair.from, "to", pair.to)
						if err := sim.PathfindTree(pair.from, pair.to); err != nil {
							log.Println("Tree pathfind from", pair.from, "to", pair.to, "failed:", err)
						}
						log.Println("SNEK pathfind from", pair.from, "to", pair.to)
						if err := sim.PathfindSNEK(pair.from, pair.to); err != nil {
							log.Println("SNEK pathfind from", pair.from, "to", pair.to, "failed:", err)
						}
					}
					wg.Done()
				}()
			}

			wg.Wait()
			log.Println("All pathfinds finished, repeating shortly...")
		}
	}()

	select {}
}

type pair struct{ from, to string }

func configureHTTPRouting(sim *simulator.Simulator) {
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
		if _, err = n.AuthenticatedConnect(conn, "websocket", router.PeerTypeRemote); err != nil {
			return
		}
		log.Printf("WebSocket peer %q connected to sim node %q\n", c.RemoteAddr(), nodeID)
	})
	http.DefaultServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles("./cmd/pineconesim/page.html"))
		nodes := sim.Nodes()
		wires := sim.Wires()
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
			AvgStretch:          "TBD",
			TreePathConvergence: "TBD",
			SNEKPathConvergence: "TBD",
			Uptime:              sim.Uptime().Round(time.Second),
		}
		if totalCount > 0 {
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
			asc, desc, table := node.DHTInfo()
			entry := Node{
				Name:    n,
				Port:    fmt.Sprintf("%d", node.ListenAddr.Port),
				Coords:  fmt.Sprintf("%v", node.Coords()),
				Key:     hex.EncodeToString(public[:2]),
				IsRoot:  node.IsRoot(),
				DHTSize: len(table),
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
				asc, desc, dht := node.Router.DHTInfo()
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
							DestinationPort: int(v.DestinationPort),
							SourcePort:      int(v.SourcePort),
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

		for range wires {
			data.PathCount++
		}

		switch r.URL.Query().Get("view") {
		case "snek":
			for id, n := range nodes {
				for id2, n2 := range nodes {
					p, _ := n.Descending()
					s, _ := n.Ascending()
					if p != nil && p.EqualTo(n2.PublicKey()) {
						data.Links = append(data.Links, Link{
							From:    id,
							To:      id2,
							Enabled: true,
						})
					}
					if s != nil && s.EqualTo(n2.PublicKey()) {
						data.Links = append(data.Links, Link{
							From:    id,
							To:      id2,
							Enabled: true,
						})
					}
				}
			}
		case "tree":
			for _, n1 := range nodes {
				if n1.IsRoot() {
					continue
				}
				r1, _ := sim.LookupPublicKey(n1.PublicKey())
				r2, _ := sim.LookupPublicKey(n1.ParentPublicKey())
				data.Links = append(data.Links, Link{
					From:    r1,
					To:      r2,
					Enabled: true,
				})
			}
		case "physical":
			fallthrough
		default:
			for a, w := range wires {
				for b, conn := range w {
					data.Links = append(data.Links, Link{
						From:    a,
						To:      b,
						Enabled: conn != nil,
					})

					// If we find any external nodes, let's show those too...
					if _, ok := nodes[b]; !ok {
						data.Nodes = append(data.Nodes, Node{
							Name:       b,
							IsExternal: true,
						})
					}
				}
			}
		}

		for n, r := range roots {
			data.Roots = append(data.Roots, Root{
				Name:       n,
				References: r * 100 / len(data.Nodes),
			})
		}
		var stretchT float64
		var stretchC int
		for a, aa := range sim.Distances() {
			for b, d := range aa {
				if a == b {
					continue
				}
				dist := Dist{
					From:     a,
					To:       b,
					Real:     "TBD",
					Observed: "TBD",
					Stretch:  "TBD",
				}
				dist.Real = fmt.Sprintf("%d", d.Real)
				if d.Observed >= d.Real {
					dist.Observed = fmt.Sprintf("%d", d.Observed)
				}
				if d.Observed >= d.Real {
					stretch := float64(1)
					if d.Real > 0 && d.Observed > 0 {
						stretch = float64(1) / float64(d.Real) * float64(d.Observed)
					}
					dist.Stretch = fmt.Sprintf("%.2f", stretch)
					stretchT += stretch
					stretchC++
				}
				//data.Dists = append(data.Dists, dist)
			}
		}
		if stretch := stretchT / float64(stretchC); stretch >= 1 {
			data.AvgStretch = fmt.Sprintf("%.2f", stretch)
		}
		_ = tmpl.Execute(w, data)
	})
}

type Node struct {
	Name        string
	Port        string
	Coords      string
	Key         string
	Root        string
	Predecessor string
	Successor   string
	IsRoot      bool
	IsExternal  bool
	DHTSize     int
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
	AvgStretch          string
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
	DestinationPort int
	SourcePort      int
	PathID          string
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
	From     string
	To       string
	Real     string
	Observed string
	Stretch  string
}
