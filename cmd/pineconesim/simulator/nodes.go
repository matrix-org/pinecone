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

	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/router/events"
)

func (sim *Simulator) Node(t string) *Node {
	sim.nodesMutex.Lock()
	defer sim.nodesMutex.Unlock()
	return sim.nodes[t]
}

func (sim *Simulator) CreateNode(t string) error {
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
	log := log.New(sim.log.Writer(), fmt.Sprintf("\033[%dmNode %s:\033[0m ", color, t), 0)
	n := &Node{
		Router:     router.NewRouter(log, sk, true),
		l:          l,
		ListenAddr: tcpaddr,
	}
	sim.nodesMutex.Lock()
	sim.nodes[t] = n
	sim.nodesMutex.Unlock()

	if sim.sockets {
		go func(n *Node) {
			for {
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
				if _, err = n.AuthenticatedConnect(c, "sim", router.PeerTypeRemote, true); err != nil {
					continue
				}
			}
		}(n)
		sim.log.Printf("Created node %q (listening on %s)\n", t, l.Addr())
	} else {
		sim.log.Printf("Created node %q\n", t)
	}
	return nil
}

func (sim *Simulator) StartNodeEventHandler(t string) {
	ch := make(chan events.Event)
	handler := eventHandler{node: t, ch: ch}
	go handler.Run(sim)
	sim.nodes[t].Subscribe(ch)

	sim.State.AddNode(t, sim.nodes[t].PublicKey().String())
}
