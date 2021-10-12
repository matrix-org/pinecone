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

type state struct {
	phony.Inbox
	r              *Router
	_peers         []*peer
	_ascending     *virtualSnakeEntry
	_descending    *virtualSnakeEntry
	_parent        *peer
	_announcements map[*peer]*rootAnnouncementWithTime
	_table         virtualSnakeTable
	_ordering      uint64 // used to order switch announcements
	_sequence      uint64 // sent in our own root updates
	_treetimer     *time.Timer
	_snaketimer    *time.Timer
	_waiting       bool // is the tree waiting to reparent?
}

func (s *state) _start() {
	s._parent = nil
	s._ascending = nil
	s._descending = nil
	s._announcements = map[*peer]*rootAnnouncementWithTime{}
	s._table = virtualSnakeTable{}
	s._ordering = 0

	s._treetimer = time.AfterFunc(announcementInterval, func() {
		s.Act(nil, s._maintainTree)
	})
	s._snaketimer = time.AfterFunc(time.Second, func() {
		s.Act(nil, s._maintainSnake)
	})

	s._maintainTreeIn(0)
	s._maintainSnakeIn(0)
}

func (s *state) _maintainTreeIn(d time.Duration) {
	if !s._treetimer.Stop() {
		select {
		case <-s._treetimer.C:
		default:
		}
	}
	s._treetimer.Reset(d)
}

func (s *state) _maintainSnakeIn(d time.Duration) {
	if !s._snaketimer.Stop() {
		select {
		case <-s._snaketimer.C:
		default:
		}
	}
	s._snaketimer.Reset(d)
}

func (s *state) _portDisconnected(peer *peer) {
	peercount := 0
	for _, p := range s._peers {
		if p != nil && p.port != 0 && p.started.Load() {
			peercount++
		}
	}
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
		return
	}

	bootstrap := false
	delete(s._announcements, peer)

	for k, v := range s._table {
		if v.Destination == peer || v.Source == peer {
			s._sendTeardownForExistingPath(peer, k.PublicKey, k.PathID)
		}
	}
	if asc := s._ascending; asc != nil && asc.Source == peer {
		s._teardownPath(s.r.local, asc.PublicKey, asc.PathID)
		bootstrap = true
	}
	if desc := s._descending; desc != nil && desc.Source == peer {
		s._teardownPath(s.r.local, desc.PublicKey, desc.PathID)
	}
	if s._parent == peer {
		bootstrap = bootstrap || s._selectNewParent()
	}
	if bootstrap {
		s._bootstrapNow()
	}
}
