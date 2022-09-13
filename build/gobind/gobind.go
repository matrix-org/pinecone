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

package gobind

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"io"
	"log"
	"net"
	"sync"
	"time"

	pineconeConnections "github.com/matrix-org/pinecone/connections"
	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/types"

	_ "golang.org/x/mobile/bind"
)

const (
	PeerTypeRemote    = pineconeRouter.PeerTypeRemote
	PeerTypeMulticast = pineconeRouter.PeerTypeMulticast
	PeerTypeBluetooth = pineconeRouter.PeerTypeBluetooth
)

type Pinecone struct {
	ctx               context.Context
	cancel            context.CancelFunc
	logger            *log.Logger
	PineconeRouter    *pineconeRouter.Router
	PineconeMulticast *pineconeMulticast.Multicast
	PineconeManager   *pineconeConnections.ConnectionManager
}

func (m *Pinecone) PeerCount(peertype int) int {
	return m.PineconeRouter.PeerCount(peertype)
}

func (m *Pinecone) SetMulticastEnabled(enabled bool) {
	if enabled {
		m.PineconeMulticast.Start()
	} else {
		m.PineconeMulticast.Stop()
		m.DisconnectType(int(pineconeRouter.PeerTypeMulticast))
	}
}

func (m *Pinecone) SetStaticPeer(uri string) {
	m.PineconeManager.RemovePeers()
	if uri != "" {
		m.PineconeManager.AddPeer(uri)
	}
}

func (m *Pinecone) DisconnectType(peertype int) {
	for _, p := range m.PineconeRouter.Peers() {
		if int(peertype) == p.PeerType {
			m.PineconeRouter.Disconnect(types.SwitchPortID(p.Port), nil)
		}
	}
}

func (m *Pinecone) DisconnectZone(zone string) {
	for _, p := range m.PineconeRouter.Peers() {
		if zone == p.Zone {
			m.PineconeRouter.Disconnect(types.SwitchPortID(p.Port), nil)
		}
	}
}

func (m *Pinecone) DisconnectPort(port int) {
	m.PineconeRouter.Disconnect(types.SwitchPortID(port), nil)
}

func (m *Pinecone) Conduit(zone string, peertype int) (*Conduit, error) {
	l, r := net.Pipe()
	conduit := &Conduit{conn: r, port: 0}
	go func() {
		conduit.portMutex.Lock()
		defer conduit.portMutex.Unlock()
	loop:
		for i := 1; i <= 10; i++ {
			var err error
			conduit.port, err = m.PineconeRouter.Connect(
				l,
				pineconeRouter.ConnectionZone(zone),
				pineconeRouter.ConnectionPeerType(peertype),
			)
			switch err {
			case io.ErrClosedPipe:
				return
			case io.EOF:
				break loop
			case nil:
				return
			default:
				time.Sleep(time.Second)
			}
		}
		_ = l.Close()
		_ = r.Close()
	}()
	return conduit, nil
}

// nolint:gocyclo
func (m *Pinecone) Start() {
	pk, sk, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())

	m.logger = log.New(BindLogger{}, "Pinecone: ", 0)
	m.logger.Println("Public key:", hex.EncodeToString(pk))

	m.PineconeRouter = pineconeRouter.NewRouter(m.logger, sk)
	m.PineconeMulticast = pineconeMulticast.NewMulticast(m.logger, m.PineconeRouter)
	m.PineconeManager = pineconeConnections.NewConnectionManager(m.PineconeRouter, nil)
}

func (m *Pinecone) Stop() {
	m.PineconeMulticast.Stop()
	_ = m.PineconeRouter.Close()
	m.cancel()
}

const MaxFrameSize = types.MaxFrameSize

type Conduit struct {
	conn      net.Conn
	port      types.SwitchPortID
	portMutex sync.Mutex
}

func (c *Conduit) Port() int {
	c.portMutex.Lock()
	defer c.portMutex.Unlock()
	return int(c.port)
}

func (c *Conduit) Read(b []byte) (int, error) {
	return c.conn.Read(b)
}

func (c *Conduit) ReadCopy() ([]byte, error) {
	var buf [65535 * 2]byte
	n, err := c.conn.Read(buf[:])
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (c *Conduit) Write(b []byte) (int, error) {
	return c.conn.Write(b)
}

func (c *Conduit) Close() error {
	return c.conn.Close()
}
