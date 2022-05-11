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
	"net"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/router/events"
	"github.com/matrix-org/pinecone/types"
)

type NeighbourInfo struct {
	PublicKey types.PublicKey
	PathID    types.VirtualSnakePathID
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

func (r *Router) NextHop(from net.Addr, frameType types.FrameType, dest net.Addr) net.Addr {
	var fromPeer *peer
	var nexthop net.Addr
	if from != nil {
		phony.Block(r.state, func() {
			fromPeer = r.state._lookupPeerForAddr(from)
		})

		if fromPeer == nil {
			r.log.Println("could not find peer info for previous peer")
			return nil
		}
	}

	var nextPeer *peer
	phony.Block(r.state, func() {
		nextPeer = r.state._nextHopsFor(fromPeer, frameType, dest)
	})

	if nextPeer != nil {
		switch (dest).(type) {
		case types.Coordinates:
			var err error
			coords := types.Coordinates{}
			phony.Block(r.state, func() {
				coords, err = nextPeer._coords()
			})

			if err != nil {
				r.log.Println("failed retrieving coords for nexthop: %w")
				return nil
			}

			nexthop = coords
		case types.PublicKey:
			nexthop = nextPeer.public
		}
	}

	return nexthop
}
