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
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"time"

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

type NodeState struct {
	PeerID           string
	Connections      map[int]string
	Parent           string
	Coords           []uint64
	Announcement     types.SwitchAnnouncement
	AnnouncementTime uint64
	AscendingPeer    string
	AscendingPathID  string
	DescendingPeer   string
	DescendingPathID string
}

// Subscribe registers a subscriber to this node's events
func (r *Router) Subscribe(ch chan<- events.Event) NodeState {
	var stateCopy NodeState
	phony.Block(r, func() {
		r._subscribers[ch] = &phony.Inbox{}
		stateCopy.PeerID = r.public.String()
		connections := map[int]string{}
		for _, p := range r.state._peers {
			if p == nil {
				continue
			}
			connections[int(p.port)] = p.public.String()
		}
		stateCopy.Connections = connections
		parent := ""
		if r.state._parent != nil {
			parent = r.state._parent.public.String()
		}
		stateCopy.Parent = parent
		coords := []uint64{}
		for _, coord := range r.Coords() {
			coords = append(coords, uint64(coord))
		}
		stateCopy.Coords = coords
		announcement := r.state._rootAnnouncement()
		stateCopy.Announcement = announcement.SwitchAnnouncement
		stateCopy.AnnouncementTime = uint64(announcement.receiveTime.UnixNano())
		asc := ""
		ascPath := ""
		if r.state._ascending != nil {
			asc = r.state._ascending.PublicKey.String()
			ascPath = hex.EncodeToString(r.state._ascending.PathID[:])
		}
		stateCopy.AscendingPeer = asc
		stateCopy.AscendingPathID = ascPath
		desc := ""
		descPath := ""
		if r.state._descending != nil {
			desc = r.state._descending.PublicKey.String()
			descPath = hex.EncodeToString(r.state._descending.PathID[:])
		}
		stateCopy.DescendingPeer = desc
		stateCopy.DescendingPathID = descPath
	})
	return stateCopy
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

func (r *Router) Ping(ctx context.Context, a net.Addr) (uint16, time.Duration, error) {
	id := a.String()
	switch dst := a.(type) {
	case types.PublicKey:
		if dst == r.public {
			return 0, 0, nil
		}
		phony.Block(r.state, func() {
			frame := getFrame()
			frame.Type = types.TypeSNEKPing
			frame.DestinationKey = dst
			frame.SourceKey = r.public
			_ = r.state._forward(r.local, frame)
		})

	case types.Coordinates:
		if dst.EqualTo(r.state.coords()) {
			return 0, 0, nil
		}
		phony.Block(r.state, func() {
			frame := getFrame()
			frame.Type = types.TypeTreePing
			frame.Destination = dst
			frame.Source = r.state._coords()
			_ = r.state._forward(r.local, frame)
		})

	default:
		return 0, 0, &net.AddrError{
			Err:  "unexpected address type",
			Addr: a.String(),
		}
	}
	start := time.Now()
	v, existing := r.pings.LoadOrStore(id, make(chan uint16))
	if existing {
		return 0, 0, fmt.Errorf("a ping to this node is already in progress")
	}
	defer r.pings.Delete(id)
	ch := v.(chan uint16)
	select {
	case <-ctx.Done():
		return 0, 0, fmt.Errorf("ping timed out")
	case hops := <-ch:
		return hops, time.Since(start), nil
	}
}
