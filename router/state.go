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
	s._treetimer.Stop()
	s._snaketimer.Stop()

	s._maintainTreeIn(announcementInterval)
	s._maintainSnakeIn(virtualSnakeMaintainInterval)
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
	bootstrap := false
	delete(s._announcements, peer)
	for k, v := range s._table {
		if v.Destination == peer || v.Source == peer {
			s._sendTeardownForExistingPath(peer, k.PublicKey, k.PathID, false)
		}
	}
	if asc := s._ascending; asc != nil && asc.Source == peer {
		s._teardownPath(peer, asc.PublicKey, asc.PathID)
		bootstrap = true
	}
	if desc := s._descending; desc != nil && desc.Source == peer {
		s._teardownPath(peer, desc.PublicKey, desc.PathID)
	}
	if s._parent == peer {
		bootstrap = bootstrap || s._selectNewParent()
	}
	if bootstrap {
		s._bootstrapNow()
	}
}
