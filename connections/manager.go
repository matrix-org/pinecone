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

package connections

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/types"
	"nhooyr.io/websocket"
)

const interval = time.Second * 5

type ConnectionManager struct {
	phony.Inbox
	ctx             context.Context
	cancel          context.CancelFunc
	router          *router.Router
	_staticPeers    map[string]struct{}
	_connectedPeers map[string]struct{}
}

func NewConnectionManager(r *router.Router) *ConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ConnectionManager{
		ctx:             ctx,
		cancel:          cancel,
		router:          r,
		_staticPeers:    map[string]struct{}{},
		_connectedPeers: map[string]struct{}{},
	}
	time.AfterFunc(interval, m.worker)
	return m
}

func (m *ConnectionManager) worker() {
	for k := range m._connectedPeers {
		delete(m._connectedPeers, k)
	}
	for _, peerInfo := range m.router.Peers() {
		m._connectedPeers[peerInfo.URI] = struct{}{}
	}

	for peer := range m._staticPeers {
		uri := peer
		if _, ok := m._connectedPeers[uri]; !ok {
			m.Act(nil, func() {
				ctx, cancel := context.WithTimeout(m.ctx, interval)
				defer cancel()
				var parent net.Conn
				switch {
				case strings.HasPrefix(uri, "ws://"):
					fallthrough
				case strings.HasPrefix(uri, "wss://"):
					c, _, err := websocket.Dial(ctx, peer, nil)
					if err != nil {
						return
					}
					parent = websocket.NetConn(m.ctx, c, websocket.MessageBinary)
				default:
					var err error
					dialer := net.Dialer{
						Timeout: interval,
					}
					parent, err = dialer.DialContext(ctx, "tcp", peer)
					if err != nil {
						return
					}
				}
				if parent == nil {
					return
				}
				_, _ = m.router.Connect(
					parent,
					router.ConnectionZone("static"),
					router.ConnectionPeerType(router.PeerTypeRemote),
					router.ConnectionURI(peer),
				)
			})
		}
	}

	select {
	case <-m.ctx.Done():
	default:
		time.AfterFunc(interval, m.worker)
	}
}

func (m *ConnectionManager) AddPeer(uri string) {
	phony.Block(m, func() {
		m._staticPeers[uri] = struct{}{}
	})
}

func (m *ConnectionManager) RemovePeer(uri string) {
	phony.Block(m, func() {
		delete(m._staticPeers, uri)
		for _, peerInfo := range m.router.Peers() {
			if peerInfo.URI == uri {
				m.router.Disconnect(types.SwitchPortID(peerInfo.Port), fmt.Errorf("removing peer"))
			}
		}
	})
}

func (m *ConnectionManager) RemovePeers() {
	phony.Block(m, func() {
		for _, peerInfo := range m.router.Peers() {
			if _, ok := m._staticPeers[peerInfo.URI]; ok {
				m.router.Disconnect(types.SwitchPortID(peerInfo.Port), fmt.Errorf("removing peer"))
			}
		}
		for uri := range m._staticPeers {
			delete(m._staticPeers, uri)
		}
	})
}
