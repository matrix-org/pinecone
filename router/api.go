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
	Port          int
	PublicKey     string
	RootPublicKey string
	PeerType      int
	Zone          string
}

/* Possible events:
   Peer Added (Port, PeerInfo)
   Peer Removed (Port, PeerInfo)
   Tree Coordinates Changed (Coordinates)
   Tree Root Announcement Changed (RootAnnouncement)
   Tree Parent Changed (PublicKey)
   Snake Table Entry Added (VirtualSnakeEntry)
   Snake Table Entry Removed (VirtualSnakeIndex?)
   Snake Descending Node Changed (NeighbourInfo)
   Snake Ascending Node Changed (NeighbourInfo)

   Events need to be processed in FIFO order for this to work in the sim.
*/

// Subscribe registers a subscriber to this node's events
func (r *Router) Subscribe(ch chan<- events.Event) {
	phony.Block(r, func() {
		r._subscribers[ch] = &phony.Inbox{}
	})
}

func (r *Router) Coords() types.Coordinates {
	return r.state.coords()
}

func (r *Router) RootPublicKey() types.PublicKey {
	var ann *rootAnnouncementWithTime
	phony.Block(r.state, func() {
		ann = r.state._rootAnnouncement()
	})
	if ann != nil {
		return ann.RootPublicKey
	}
	return r.public
}

func (r *Router) ParentPublicKey() types.PublicKey {
	var parent *peer
	phony.Block(r.state, func() {
		parent = r.state._parent
	})
	if parent == nil {
		return r.public
	}
	return parent.public
}

func (r *Router) IsRoot() bool {
	return r.RootPublicKey() == r.public
}

func (r *Router) DHTInfo() (asc, desc *NeighbourInfo, table map[virtualSnakeIndex]virtualSnakeEntry, stale int) {
	table = map[virtualSnakeIndex]virtualSnakeEntry{}
	phony.Block(r.state, func() {
		ann := r.state._rootAnnouncement()
		if a := r.state._ascending; a != nil {
			asc = &NeighbourInfo{
				PublicKey: a.Origin,
				PathID:    a.PathID,
			}
		}
		if d := r.state._descending; d != nil {
			desc = &NeighbourInfo{
				PublicKey: d.PublicKey,
				PathID:    d.PathID,
			}
		}
		dupes := map[types.PublicKey]int{}
		for k := range r.state._table {
			dupes[k.PublicKey]++
		}
		for k, v := range r.state._table {
			table[k] = *v
			if c, ok := dupes[k.PublicKey]; ok && c > 1 {
				stale += 1
			}
			switch {
			case v.Root.RootPublicKey != ann.RootPublicKey:
				stale++
			}
		}
	})
	return
}

func (r *Router) Descending() *NeighbourInfo {
	_, desc, _, _ := r.DHTInfo()
	return desc
}

func (r *Router) Ascending() *NeighbourInfo {
	asc, _, _, _ := r.DHTInfo()
	return asc
}

func (r *Router) Peers() []PeerInfo {
	peers := make([]PeerInfo, 0, portCount)
	phony.Block(r.state, func() {
		for _, p := range r.state._peers {
			if p == nil || !p.started.Load() {
				continue
			}
			info := PeerInfo{
				Port:      int(p.port),
				PeerType:  p.peertype,
				PublicKey: p.public.String(),
				Zone:      p.zone,
			}
			if r.state._announcements[p] != nil {
				info.RootPublicKey = r.state._announcements[p].RootPublicKey.String()
			}
			if info.RootPublicKey == "" {
				info.RootPublicKey = r.public.String()
			}
			peers = append(peers, info)
		}
	})
	return peers
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
