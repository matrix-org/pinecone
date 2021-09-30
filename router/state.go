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

	s._becomeRoot()

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
	// SNEK routing
	case types.TypeVirtualSnake, types.TypeVirtualSnakeBootstrap:
		phony.Block(s, func() {
			nexthops = s._nextHopsSNEK(from, frame, frame.Type == types.TypeVirtualSnakeBootstrap)
		})

	// Tree routing
	case types.TypeVirtualSnakeBootstrapACK, types.TypeVirtualSnakeSetup:
		fallthrough
	case types.TypeGreedy:
		phony.Block(s, func() {
			nexthops = s._nextHopsTree(from, frame)
		})
	}
	return nexthops
}

func (s *state) _portDisconnected(peer *peer) {
	delete(s._announcements, peer)

	if s._parent == peer {
		s._selectNewParent()
	}

	// TODO: handle case that parent disappeared
	// TODO: teardown transitive paths
}
