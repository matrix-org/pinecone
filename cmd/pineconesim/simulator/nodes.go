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
	"net"

	"github.com/matrix-org/pinecone/router"
)

func (sim *Simulator) Node(t string) *Node {
	sim.nodesMutex.Lock()
	defer sim.nodesMutex.Unlock()
	return sim.nodes[t]
}

func (sim *Simulator) CreateNode(t string) error {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("net.Listen: %w", err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return fmt.Errorf("net.SplitHostPort: %w", err)
	}

	pk, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("ed25519.GenerateKey: %w", err)
	}

	n := &Node{
		Router:     router.NewRouter(sim.log, t, sk, pk, sim),
		l:          l,
		ListenPort: port,
	}
	sim.nodesMutex.Lock()
	sim.nodes[t] = n
	sim.nodesMutex.Unlock()

	go func(n *Node) {
		for {
			c, err := l.Accept()
			if err != nil {
				continue
			}
			if _, err = n.AuthenticatedConnect(c, "sim", router.PeerTypeRemote); err != nil {
				continue
			}
		}
	}(n)

	sim.log.Printf("Created node %q (listening on %s)\n", t, l.Addr())
	//sim.log.Printf("Created node %q\n", t)
	return nil
}
