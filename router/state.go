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
	"go.uber.org/atomic"
)

type FilterFn func(from types.PublicKey, f *types.Frame) bool

// NOTE: Functions prefixed with an underscore (_) are only safe to be called
// from the actor that owns them, in order to prevent data races.

// state is an actor that owns all of the mutable state for the Pinecone router.
type state struct {
	phony.Inbox
	r              *Router
	_peers         []*peer            // All switch ports, connected and disconnected
	_descending    *virtualSnakeEntry // Next descending node in keyspace
	_parent        *peer              // Our chosen parent in the tree
	_announcements announcementTable  // Announcements received from our peers
	_table         virtualSnakeTable  // Virtual snake DHT entries
	_ordering      uint64             // Used to order incoming tree announcements
	_sequence      uint64             // Used to sequence our root tree announcements
	_treetimer     *time.Timer        // Tree maintenance timer
	_snaketimer    *time.Timer        // Virtual snake maintenance timer
	_lastbootstrap time.Time          // When did we last bootstrap?
	_waiting       bool               // Is the tree waiting to reparent?
	_filterPacket  FilterFn           // Function called when forwarding packets
}

// _start resets the state and starts tree and virtual snake maintenance.
func (s *state) _start() {
	s._setParent(nil)
	s._setDescendingNode(nil)

	s._ordering = 0
	s._waiting = false

	s._announcements = make(announcementTable, portCount)
	s._table = virtualSnakeTable{}

	if s._treetimer == nil {
		s._treetimer = time.AfterFunc(announcementInterval, func() {
			s.Act(nil, s._maintainTree)
		})
	}

	if s._snaketimer == nil {
		s._snaketimer = time.AfterFunc(time.Second, func() {
			s.Act(nil, s._maintainSnake)
		})
	}

	s._maintainTreeIn(0)
	s._maintainSnakeIn(0)
}

// _maintainTreeIn resets the tree maintenance timer to the specified
// duration.
func (s *state) _maintainTreeIn(d time.Duration) {
	if !s._treetimer.Stop() {
		select {
		case <-s._treetimer.C:
		default:
		}
	}
	s._treetimer.Reset(d)
}

// _maintainSnakeIn resets the virtual snake maintenance timer to the
// specified duration.
func (s *state) _maintainSnakeIn(d time.Duration) {
	if !s._snaketimer.Stop() {
		select {
		case <-s._snaketimer.C:
		default:
		}
	}
	s._snaketimer.Reset(d)
}

// _addPeer creates a new Peer and adds it to the switch in the next available port
func (s *state) _addPeer(conn net.Conn, public types.PublicKey, uri ConnectionURI, zone ConnectionZone, peertype ConnectionPeerType, keepalives bool) (types.SwitchPortID, error) {
	var new *peer
	for i, p := range s._peers {
		if i == 0 || p != nil {
			// Port 0 is reserved for the local router.
			// Already allocated ports should be ignored.
			continue
		}
		ctx, cancel := context.WithCancel(s.r.context)
		queues := uint16(trafficBuffer)
		if peertype == ConnectionPeerType(PeerTypeBluetooth) {
			queues = 16
		}
		new = &peer{
			router:     s.r,
			port:       types.SwitchPortID(i),
			conn:       conn,
			public:     public,
			uri:        uri,
			zone:       zone,
			peertype:   peertype,
			keepalives: keepalives,
			context:    ctx,
			cancel:     cancel,
			proto:      newFIFOQueue(fifoNoMax, s.r.log),
			traffic:    newFairFIFOQueue(queues, s.r.log),
		}
		s._peers[i] = new
		s.r.log.Println("Connected to peer", new.public.String(), "on port", new.port)
		v, _ := s.r.active.LoadOrStore(hex.EncodeToString(new.public[:])+string(zone), atomic.NewUint64(0))
		v.(*atomic.Uint64).Inc()
		new.proto.push(s.r.state._rootAnnouncement().forPeer(new))
		new.started.Store(true)
		new.reader.Act(nil, new._read)
		new.writer.Act(nil, new._write)

		s.r.Act(nil, func() {
			s.r._publish(events.PeerAdded{Port: types.SwitchPortID(i), PeerID: new.public.String()})
		})
		return types.SwitchPortID(i), nil
	}

	return 0, fmt.Errorf("no free switch ports")
}

// _removePeer removes the Peer from the specified switch port
func (s *state) _removePeer(port types.SwitchPortID) {
	peerID := s._peers[port].public.String()
	s._peers[port] = nil
	s.r.Act(nil, func() {
		s.r._publish(events.PeerRemoved{Port: port, PeerID: peerID})
	})
}

func (s *state) _setParent(peer *peer) {
	s._parent = peer

	s.r.Act(nil, func() {
		peerID := ""
		if peer != nil {
			peerID = peer.public.String()
		}

		s.r._publish(events.TreeParentUpdate{PeerID: peerID})
	})
}

func (s *state) _setDescendingNode(node *virtualSnakeEntry) {
	switch {
	case s._descending == nil || node == nil:
		fallthrough
	case s._descending != nil && node != nil && s._descending.PublicKey != node.PublicKey:
		s._bootstrapSoon()
	}

	s._descending = node

	s.r.Act(nil, func() {
		peerID := ""
		if node != nil {
			peerID = node.PublicKey.String()
		}

		s.r._publish(events.SnakeDescUpdate{PeerID: peerID})
	})
}

func (s *state) _addRouteEntry(index virtualSnakeIndex, entry *virtualSnakeEntry) {
	s._table[index] = entry

	s.r.Act(nil, func() {
		s.r._publish(events.SnakeEntryAdded{EntryID: index.PublicKey.String(), PeerID: entry.Source.public.String()})
	})
}

func (s *state) _removeRouteEntry(index virtualSnakeIndex) {
	delete(s._table, index)

	s.r.Act(nil, func() {
		s.r._publish(events.SnakeEntryRemoved{EntryID: index.PublicKey.String()})
	})
}

// _portDisconnected is called when a peer disconnects.
func (s *state) _portDisconnected(peer *peer) {
	peercount := 0

	// Work out how many peers are connected now that this peer has
	// disconnected.
	for _, p := range s._peers {
		if p != nil && p.port != 0 && p.started.Load() {
			peercount++
		}
	}

	// If there are no peers connected anymore, reset the tree and virtual
	// snake state. When we connect to a peer in the future, we will do so
	// with a blank slate.
	if peercount == 0 {
		s._start()
		return
	}

	// Delete the last tree announcement that we received from this peer.
	delete(s._announcements, peer)

	// Scan the local routing table for any routes that transited this now-dead
	// peering and remove them from the routing table.
	for k, v := range s._table {
		if v.Source == peer || v.Destination == peer {
			s._removeRouteEntry(k)
		}
	}

	// If the descending path was lost because it went via the now-dead
	// peering then clear that path and wait for another incoming setup.
	if desc := s._descending; desc != nil && desc.Source == peer {
		s._setDescendingNode(nil)
	}

	// If the peer that died was our chosen tree parent, then we will need to
	// select a new parent. If we successfully choose a new parent (as in, we
	// don't end up promoting ourselves to a root) then we will also need to
	// send a new bootstrap into the network.
	if s._parent == peer && s._selectNewParent() {
		s._bootstrapSoon()
	}
}

// _lookupPeerForAddr finds and returns the peer corresponding to the provided
// net.Addr if such a peer exists.
func (s *state) _lookupPeerForAddr(addr net.Addr) *peer {
	var result *peer

	for _, p := range s._peers {
		if p == nil || !p.started.Load() {
			continue
		}

		switch fromAddr := addr.(type) {
		case types.Coordinates:
			coords, err := p._coords()
			if err == nil && fromAddr.EqualTo(coords) {
				result = p
				break
			}
		case types.PublicKey:
			if fromAddr == p.public {
				result = p
				break
			}
		}
	}

	return result
}
