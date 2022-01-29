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
	"fmt"
	"hash/crc32"
	"log"
	"net"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator/adversary"
	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/router/events"
)

func (sim *Simulator) Node(t string) *Node {
	sim.nodesMutex.Lock()
	defer sim.nodesMutex.Unlock()
	return sim.nodes[t]
}

func (sim *Simulator) CreateNode(t string, nodeType APINodeType) error {
	if _, ok := sim.nodes[t]; ok {
		return fmt.Errorf("%s already exists!", t)
	}

	var l *net.TCPListener
	var tcpaddr *net.TCPAddr
	if sim.sockets {
		var err error
		l, err = net.ListenTCP("tcp", &net.TCPAddr{
			IP:   net.IPv4zero,
			Port: 0,
		})
		var ok bool
		tcpaddr, ok = l.Addr().(*net.TCPAddr)
		if !ok {
			panic("Not tcpaddr")
		}
		if err != nil {
			return fmt.Errorf("net.Listen: %w", err)
		}
	}
	_, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("ed25519.GenerateKey: %w", err)
	}
	crc := crc32.ChecksumIEEE([]byte(t))
	color := 31 + (crc % 6)
	logger := log.New(sim.log.Writer(), fmt.Sprintf("\033[%dmNode %s:\033[0m ", color, t), 0)

	n := &Node{
		SimRouter:  sim.routerCreationMap[nodeType](logger, sk, true),
		l:          l,
		ListenAddr: tcpaddr,
	}
	sim.nodesMutex.Lock()
	sim.nodes[t] = n
	sim.nodesMutex.Unlock()

	if sim.sockets {
		quit := make(chan bool)
		go func(quit <-chan bool, n *Node) {
			for {
				select {
				case <-quit:
					return
				default:
					n.l.SetDeadline(time.Now().Add(time.Duration(500) * time.Millisecond))
					c, err := n.l.AcceptTCP()
					if err != nil {
						continue
					}
					if err := c.SetNoDelay(true); err != nil {
						panic(err)
					}
					if err := c.SetLinger(0); err != nil {
						panic(err)
					}
					if _, err = n.Connect(
						c,
						router.ConnectionPeerType(router.PeerTypeRemote),
						router.ConnectionKeepalives(true),
					); err != nil {
						continue
					}
				}
			}
		}(quit, n)

		sim.nodeRunnerChannelsMutex.Lock()
		sim.nodeRunnerChannels[t] = append(sim.nodeRunnerChannels[t], quit)
		sim.nodeRunnerChannelsMutex.Unlock()

		sim.log.Printf("Created node %q (listening on %s)\n", t, l.Addr())
	} else {
		sim.log.Printf("Created node %q\n", t)
	}
	return nil
}

func (sim *Simulator) StartNodeEventHandler(t string, nodeType APINodeType) {
	ch := make(chan events.Event)
	handler := eventHandler{node: t, ch: ch}
	quit := make(chan bool)
	nodeState := sim.nodes[t].Subscribe(ch)

	sim.nodeRunnerChannelsMutex.Lock()
	sim.nodeRunnerChannels[t] = append(sim.nodeRunnerChannels[t], quit)
	sim.nodeRunnerChannelsMutex.Unlock()

	phony.Block(sim.State, func() { sim.State._addNode(t, sim.nodes[t].PublicKey().String(), nodeType, nodeState) })
	go handler.Run(quit, sim)
}

func (sim *Simulator) RemoveNode(node string) {
	// Stop all the goroutines running for this node
	sim.nodeRunnerChannelsMutex.Lock()
	for _, quitChan := range sim.nodeRunnerChannels[node] {
		quitChan <- true
	}
	delete(sim.nodeRunnerChannels, node)
	sim.nodeRunnerChannelsMutex.Unlock()

	sim.DisconnectAllPeers(node)

	// Remove the node from the simulators list of nodes
	sim.nodesMutex.Lock()
	delete(sim.nodes, node)
	sim.nodesMutex.Unlock()

	phony.Block(sim.State, func() { sim.State._removeNode(node) })
}

func (sim *Simulator) ConfigureFilterDefaults(node string, rates adversary.DropRates) {
	if node, exists := sim.Nodes()[node]; exists {
		node.ConfigureFilterDefaults(rates)
	}
}

func (sim *Simulator) ConfigureFilterPeer(node string, peer string, rates adversary.DropRates) {
	peerNode, exists := sim.Nodes()[peer]
	if !exists {
		log.Println("Failed configuring filters for peer. Key too long")
		return
	}

	if node, exists := sim.Nodes()[node]; exists {
		node.ConfigureFilterPeer(peerNode.PublicKey(), rates)
	}
}

func createDefaultRouter(log *log.Logger, sk ed25519.PrivateKey, debug bool) SimRouter {
	rtr := &DefaultRouter{
		router.NewRouter(log, sk, debug),
	}
	return rtr
}

func createAdversaryRouter(log *log.Logger, sk ed25519.PrivateKey, debug bool) SimRouter {
	return adversary.NewAdversaryRouter(log, sk, debug)
}
