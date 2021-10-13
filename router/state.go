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
	"time"

	"github.com/Arceliar/phony"
)

// NOTE: Functions prefixed with an underscore (_) are only safe to be called
// from the actor that owns them, in order to prevent data races.

// state is an actor that owns all of the mutable state for the Pinecone router.
type state struct {
	phony.Inbox
	r               *Router
	_peers          []*peer            // All switch ports, connected and disconnected
	_ascending      *virtualSnakeEntry // Next ascending node in keyspace
	_descending     *virtualSnakeEntry // Next descending node in keyspace
	_parent         *peer              // Our chosen parent in the tree
	_announcements  announcementTable  // Announcements received from our peers
	_table          virtualSnakeTable  // Virtual snake DHT entries
	_ordering       uint64             // Used to order incoming tree announcements
	_sequence       uint64             // Used to sequence our root tree announcements
	_treetimer      *time.Timer        // Tree maintenance timer
	_snaketimer     *time.Timer        // Virtual snake maintenance timer
	_bootstraptimer *time.Timer        // Virtual snake bootstrap timer
	_waiting        bool               // Is the tree waiting to reparent?
}

// _start resets the state and starts tree and virtual snake maintenance.
func (s *state) _start() {
	s._parent = nil
	s._ascending = nil
	s._descending = nil
	s._announcements = make(announcementTable, PortCount)
	s._table = virtualSnakeTable{}
	s._ordering = 0

	s._treetimer = time.AfterFunc(announcementInterval, func() {
		s.Act(nil, s._maintainTree)
	})
	s._snaketimer = time.AfterFunc(time.Second, func() {
		s.Act(nil, s._maintainSnake)
	})
	s._bootstraptimer = time.AfterFunc(time.Second, func() {
		s.Act(nil, s._bootstrapNow)
	})

	s._maintainTreeIn(0)
	s._bootstrapIn(0)
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

// _bootstrapIn resets the virtual snake bootstrap timer to the
// specified duration.
func (s *state) _bootstrapIn(d time.Duration) {
	if !s._bootstraptimer.Stop() {
		select {
		case <-s._bootstraptimer.C:
		default:
		}
	}
	s._bootstraptimer.Reset(d)
}

// _portDisconnected is called when a peer disconnects.
func (s *state) _portDisconnected(peer *peer) {
	peercount := 0
	bootstrap := false

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
		s._parent = nil
		s._ascending = nil
		s._descending = nil
		for k := range s._announcements {
			delete(s._announcements, k)
		}
		for k := range s._table {
			delete(s._table, k)
		}
		s._ordering = 0
		return
	}

	// Delete the last tree announcement that we received from this peer.
	delete(s._announcements, peer)

	// Scan the local DHT table for any routes that transited this now-dead
	// peering. If we find any then we need to send teardowns in the opposite
	// direction, so that nodes further along the path will learn that the
	// path was broken.
	for k, v := range s._table {
		if v.Destination == peer || v.Source == peer {
			s._sendTeardownForExistingPath(peer, k.PublicKey, k.PathID)
		}
	}

	// If the ascending path was also lost because it went via the now-dead
	// peering then clear that path (although we can't send a teardown) and
	// then bootstrap again.
	if asc := s._ascending; asc != nil && asc.Source == peer {
		s._teardownPath(s.r.local, asc.PublicKey, asc.PathID)
		bootstrap = true
	}

	// If the descending path was lost because it went via the now-dead
	// peering then clear that path (although we can't send a teardown) and
	// wait for another incoming setup.
	if desc := s._descending; desc != nil && desc.Source == peer {
		s._teardownPath(s.r.local, desc.PublicKey, desc.PathID)
	}

	// If the peer that died was our chosen tree parent, then we will need to
	// select a new parent. If we successfully choose a new parent (as in, we
	// don't end up promoting ourselves to a root) then we will also need to
	// send a new bootstrap into the network.
	if s._parent == peer {
		bootstrap = bootstrap || s._selectNewParent()
	}

	if bootstrap {
		s._bootstrapIn(0)
	}
}
