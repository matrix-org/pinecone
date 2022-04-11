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
	"math"
	"net"
	"net/http"
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
	client          *http.Client
	ws              *websocket.DialOptions
	_staticPeers    map[string]*connectionAttempts
	_connectedPeers map[string]struct{}
}

type connectionAttempts struct {
	attempts float64
	next     time.Time
}

func NewConnectionManager(r *router.Router, client *http.Client) *ConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ConnectionManager{
		ctx:    ctx,
		cancel: cancel,
		router: r,
		client: client,
		ws: &websocket.DialOptions{
			HTTPClient: client,
		},
		_staticPeers:    map[string]*connectionAttempts{},
		_connectedPeers: map[string]struct{}{},
	}
	if m.ws.HTTPClient == nil {
		m.ws.HTTPClient = http.DefaultClient
	}
	time.AfterFunc(interval, m._worker)
	return m
}

func (m *ConnectionManager) _connect(uri string) {
	result := func(err error) {
		attempts := m._staticPeers[uri]
		if attempts == nil {
			return
		}
		if err != nil {
			attempts.attempts++
			until := time.Second * time.Duration(math.Exp2(attempts.attempts))
			if until > time.Hour {
				until = time.Hour
			}
			attempts.next = time.Now().Add(until)
		} else {
			attempts.attempts = 0
			attempts.next = time.Now()
		}
	}
	ctx, cancel := context.WithTimeout(m.ctx, interval)
	defer cancel()
	var parent net.Conn
	switch {
	case strings.HasPrefix(uri, "ws://"):
		fallthrough
	case strings.HasPrefix(uri, "wss://"):
		c, _, err := websocket.Dial(ctx, uri, m.ws)
		if err != nil {
			result(err)
			return
		}
		parent = websocket.NetConn(m.ctx, c, websocket.MessageBinary)
	default:
		var err error
		dialer := net.Dialer{
			Timeout: interval,
		}
		parent, err = dialer.DialContext(ctx, "tcp", uri)
		if err != nil {
			result(err)
			return
		}
	}
	if parent == nil {
		result(fmt.Errorf("no parent connection"))
		return
	}
	_, err := m.router.Connect(
		parent,
		router.ConnectionZone("static"),
		router.ConnectionPeerType(router.PeerTypeRemote),
		router.ConnectionURI(uri),
	)
	result(err)
}

func (m *ConnectionManager) _worker() {
	for k := range m._connectedPeers {
		delete(m._connectedPeers, k)
	}
	for _, peerInfo := range m.router.Peers() {
		m._connectedPeers[peerInfo.URI] = struct{}{}
	}

	for peer, attempts := range m._staticPeers {
		if _, ok := m._connectedPeers[peer]; !ok && time.Now().After(attempts.next) {
			uri := peer
			m.Act(nil, func() {
				m._connect(uri)
			})
		}
	}

	select {
	case <-m.ctx.Done():
	default:
		time.AfterFunc(interval, m._worker)
	}
}

func (m *ConnectionManager) AddPeer(uri string) {
	phony.Block(m, func() {
		if _, existing := m._staticPeers[uri]; existing {
			return
		}
		m._staticPeers[uri] = &connectionAttempts{
			attempts: 0,
			next:     time.Now(),
		}
		m._connect(uri)
	})
}

func (m *ConnectionManager) RemovePeer(uri string) {
	phony.Block(m, func() {
		if _, existing := m._staticPeers[uri]; !existing {
			return
		}
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
