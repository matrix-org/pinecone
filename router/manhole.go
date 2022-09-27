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

package router

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
)

type manholeResponse struct {
	Public types.PublicKey          `json:"public_key"`
	Coords types.Coordinates        `json:"coords"`
	Root   *types.Root              `json:"root"`
	Parent *peer                    `json:"parent"`
	Peers  map[string][]manholePeer `json:"peers"`
	SNEK   struct {
		Descending *virtualSnakeEntry   `json:"descending"`
		Paths      []*virtualSnakeEntry `json:"paths"`
	} `json:"snek"`
	CoordCache map[string]types.Coordinates `json:"coords_cache"`
}

type manholePeer struct {
	Coords       types.Coordinates  `json:"coords,omitempty"`
	Order        uint64             `json:"order,omitempty"`
	Port         types.SwitchPortID `json:"port"`
	PeerType     ConnectionPeerType `json:"type,omitempty"`
	PeerZone     ConnectionZone     `json:"zone,omitempty"`
	PeerURI      ConnectionURI      `json:"uri,omitempty"`
	RXProto      uint64             `json:"rx_proto_bytes"`
	TXProto      uint64             `json:"tx_proto_bytes"`
	RXTraffic    uint64             `json:"rx_traffic_bytes"`
	TXTraffic    uint64             `json:"tx_traffic_bytes"`
	ProtoQueue   queue              `json:"proto_queue"`
	TrafficQueue queue              `json:"traffic_queue"`
}

func (r *Router) ManholeHandler(w http.ResponseWriter, req *http.Request) {
	response := manholeResponse{
		Public: r.public,
		Peers:  map[string][]manholePeer{},
	}
	phony.Block(r.state, func() {
		response.Public = r.public
		response.Coords = r.state._coords()
		response.Parent = r.state._parent
		if rootAnn := r.state._rootAnnouncement(); rootAnn != nil {
			response.Root = &rootAnn.Root
		}
		response.CoordCache = map[string]types.Coordinates{}
		for k, v := range r.state._coordsCache {
			if time.Since(v.lastSeen) > coordsCacheLifetime {
				continue
			}
			response.CoordCache[k.String()] = v.coordinates
		}
		for _, p := range r.state._peers {
			if p == nil || !p.started.Load() {
				continue
			}
			info := manholePeer{
				Port:         p.port,
				PeerType:     p.peertype,
				PeerZone:     p.zone,
				PeerURI:      p.uri,
				ProtoQueue:   p.proto,
				TrafficQueue: p.traffic,
			}
			phony.Block(&p.statistics, func() {
				info.RXProto, info.RXTraffic = p.statistics._bytesRxProto, p.statistics._bytesRxTraffic
				info.TXProto, info.TXTraffic = p.statistics._bytesTxProto, p.statistics._bytesTxTraffic
			})
			if ann := r.state._announcements[p]; ann != nil {
				info.Coords = ann.Coords()
				info.Order = ann.receiveOrder
			}
			public := p.public.String()
			response.Peers[public] = append(response.Peers[public], info)
		}
		response.SNEK.Descending = r.state._descending
		for _, p := range r.state._table {
			response.SNEK.Paths = append(response.SNEK.Paths, p)
		}
	})
	for _, p := range response.Peers {
		sort.Slice(p, func(i, j int) bool {
			return p[i].Order < p[j].Order
		})
	}
	sort.Slice(response.SNEK.Paths, func(i, j int) bool {
		return response.SNEK.Paths[i].PublicKey.CompareTo(response.SNEK.Paths[j].PublicKey) < 0
	})
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(response); err != nil {
		w.WriteHeader(500)
		return
	}
}
