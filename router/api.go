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

//go:build !minimal
// +build !minimal

package router

import (
	"encoding/hex"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/router/events"
	"github.com/matrix-org/pinecone/types"
)

type NeighbourInfo struct {
	PublicKey types.PublicKey
}

type PeerInfo struct {
	URI       string
	Port      int
	PublicKey string
	PeerType  int
	Zone      string
}

// Subscribe registers a subscriber to this node's events
func (r *Router) Subscribe(ch chan<- events.Event) {
	phony.Block(r, func() {
		r._subscribers[ch] = &phony.Inbox{}
	})
}

func (r *Router) Coords() types.Coordinates {
	return r.state.coords()
}

func (r *Router) Peers() []PeerInfo {
	var infos []PeerInfo
	phony.Block(r.state, func() {
		for _, p := range r.state._peers {
			if p == nil {
				continue
			}
			infos = append(infos, PeerInfo{
				URI:       string(p.uri),
				Port:      int(p.port),
				PublicKey: hex.EncodeToString(p.public[:]),
				PeerType:  int(p.peertype),
				Zone:      string(p.zone),
			})
		}
	})
	return infos
}

func (r *Router) EnableHopLimiting() {
	r._hopLimiting.Store(true)
}

func (r *Router) DisableHopLimiting() {
	r._hopLimiting.Store(false)
}
