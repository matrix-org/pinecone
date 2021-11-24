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
	"fmt"
	"net"
	"time"

	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/util"
)

func (sim *Simulator) ConnectNodes(a, b string) error {
	sim.nodesMutex.RLock()
	na := sim.nodes[a]
	nb := sim.nodes[b]
	sim.nodesMutex.RUnlock()
	if na == nil || nb == nil {
		return fmt.Errorf("invalid node pair")
	}

	sim.wiresMutex.RLock()
	wa := sim.wires[a][b]
	wb := sim.wires[b][a]
	sim.wiresMutex.RUnlock()
	if wa != nil || wb != nil {
		return fmt.Errorf("already connected")
	}

	register := func(conn net.Conn) {
		sim.wiresMutex.Lock()
		defer sim.wiresMutex.Unlock()
		if sim.wires[a] == nil {
			sim.wires[a] = map[string]net.Conn{}
		}
		sim.wires[a][b] = conn
	}

	if sim.sockets {
		c, err := net.DialTCP(na.l.Addr().Network(), nil, na.ListenAddr)
		if err != nil {
			return fmt.Errorf("net.Dial: %w", err)
		}
		if err := c.SetNoDelay(true); err != nil {
			panic(err)
		}
		sc := &util.SlowConn{Conn: c, ReadJitter: 5 * time.Millisecond}
		if _, err := nb.Connect(
			sc,
			router.ConnectionKeepalives(false),
			router.PeerTypeRemote,
		); err != nil {
			return fmt.Errorf("nb.AuthenticatedConnect: %w", err)
		}
		register(sc)
	} else {
		pa, pb := net.Pipe()
		pa = &util.SlowConn{Conn: pa, ReadJitter: 5 * time.Millisecond}
		pb = &util.SlowConn{Conn: pb, ReadJitter: 5 * time.Millisecond}
		go func() {
			if _, err := na.Connect(
				pa,
				router.ConnectionPublicKey(nb.PublicKey()),
				router.ConnectionKeepalives(false),
				router.PeerTypeRemote,
			); err != nil {
				return
			}
		}()
		go func() {
			if _, err := nb.Connect(
				pb,
				router.ConnectionPublicKey(na.PublicKey()),
				router.ConnectionKeepalives(false),
				router.PeerTypeRemote,
			); err != nil {
				return
			}
		}()
		register(pa)
	}

	sim.log.Printf("Connected node %q to node %q\n", a, b)
	return nil
}

func (sim *Simulator) DisconnectNodes(a, b string) error {
	sim.wiresMutex.RLock()
	wire := sim.wires[a][b]
	if wire == nil {
		wire = sim.wires[b][a]
	}
	sim.wiresMutex.RUnlock()
	if wire == nil {
		return fmt.Errorf("no wire found")
	}
	sim.wiresMutex.Lock()
	sim.wires[a][b] = nil
	sim.wiresMutex.Unlock()
	return wire.Close()
}
