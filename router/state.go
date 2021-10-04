package router

import (
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
)

type state struct {
	phony.Inbox
	r              *Router
	_ascending     *virtualSnakeNeighbour
	_descending    *virtualSnakeNeighbour
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

func (s *state) nextHopsFor(from *peer, frame *types.Frame) []*peer {
	var nexthops []*peer
	switch frame.Type {
	case types.TypeVirtualSnakeTeardown:
		// Teardowns have special logic so we do nothing with them
		return nil

	// SNEK routing
	case types.TypeVirtualSnake, types.TypeVirtualSnakeBootstrap, types.TypeSNEKPing, types.TypeSNEKPong:
		phony.Block(s, func() {
			nexthops = s._nextHopsSNEK(from, frame, frame.Type == types.TypeVirtualSnakeBootstrap)
		})

	// Tree routing
	case types.TypeVirtualSnakeBootstrapACK, types.TypeVirtualSnakeSetup:
		fallthrough
	case types.TypeGreedy, types.TypeTreePing, types.TypeTreePong:
		phony.Block(s, func() {
			nexthops = s._nextHopsTree(from, frame)
		})

	// Source routing
	case types.TypeSource:
		if len(frame.Destination) == 0 {
			return []*peer{s.r.local}
		}
		var nexthop *peer
		phony.Block(s.r, func() {
			port := s.r._peers[frame.Destination[0]]
			if frame.Destination[0] == from.port {
				return
			}
			frame.Destination = frame.Destination[1:]
			if from != nexthop && nexthop != nil && nexthop.started.Load() {
				nexthop = port
			}
		})
		if nexthop != nil {
			return []*peer{nexthop}
		}
	}
	return nexthops
}

func (s *state) _portDisconnected(peer *peer) {
	delete(s._announcements, peer)

	if s._parent == peer {
		s._selectNewParent()
	}

	if asc := s._ascending; asc != nil && asc.Port == peer {
		s._teardownPath(peer, asc.PublicKey, asc.PathID)
		defer s._bootstrapNow()
	}
	if desc := s._descending; desc != nil && desc.Port == peer {
		s._teardownPath(peer, desc.PublicKey, desc.PathID)
	}
	for k, v := range s._table {
		if v.Destination == peer || v.Source == peer {
			s._teardownPath(peer, k.PublicKey, k.PathID)
		}
	}
}
