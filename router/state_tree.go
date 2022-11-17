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
	"fmt"
	"math"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/router/events"
	"github.com/matrix-org/pinecone/types"
)

// NOTE: Functions prefixed with an underscore (_) are only safe to be called
// from the actor that owns them, in order to prevent data races.

type announcementTable map[*peer]*rootAnnouncementWithTime

// _maintainTree sends out root announcements if we are
// considering ourselves to be a root node.
func (s *state) _maintainTree() {
	select {
	case <-s.r.context.Done():
		return
	default:
		defer s._maintainTreeIn(announcementInterval)
	}

	// If we don't have a parent then we are acting as if we are a root node,
	// so we need to send tree announcements to our peers. In each instance,
	// we will update the sequence number so that downstream nodes know that
	// it's a new update.
	if s._parent == nil {
		s._sequence++
		s._sendTreeAnnouncements()
	}
}

type rootAnnouncementWithTime struct {
	types.SwitchAnnouncement
	receiveTime  time.Time // when did we receive the update?
	receiveOrder uint64    // the relative order that the update was received
}

// forPeer generates a frame with a signed root announcement for the given
// peer.
func (a *rootAnnouncementWithTime) forPeer(p *peer) *types.Frame {
	if p == nil || p.port == 0 {
		panic("trying to send announcement to nil port or port 0")
	}
	announcement := a.SwitchAnnouncement
	announcement.Signatures = append([]types.SignatureWithHop{}, a.Signatures...)
	for _, sig := range announcement.Signatures {
		if p.router.public == sig.PublicKey {
			// For some reason the announcement that we want to send already
			// includes our signature. This shouldn't really happen but if we
			// did send it, other nodes would end up ignoring the announcement
			// anyway since it would appear to be a routing loop.
			panic("trying to send announcement with loop")
		}
	}
	// Sign the announcement.
	if err := announcement.Sign(p.router.private[:], p.port); err != nil {
		panic("failed to sign switch announcement: " + err.Error())
	}
	frame := getFrame()
	frame.Type = types.TypeTreeAnnouncement
	n, err := announcement.MarshalBinary(frame.Payload[:cap(frame.Payload)])
	if err != nil {
		panic("failed to marshal switch announcement: " + err.Error())
	}
	frame.Payload = frame.Payload[:n]
	return frame
}

// _rootAnnouncement returns the latest root announcement from our parent.
// If we are the root, or the announcement from the parent has expired, we
// will instead return a root update with ourselves as the root.
func (s *state) _rootAnnouncement() *rootAnnouncementWithTime {
	if s._parent == nil || s._announcements[s._parent] == nil {
		return &rootAnnouncementWithTime{
			SwitchAnnouncement: types.SwitchAnnouncement{
				Root: types.Root{
					RootPublicKey: s.r.public,
					RootSequence:  types.Varu64(s._sequence),
				},
			},
		}
	}
	return s._announcements[s._parent]
}

// coords returns our tree coordinates, or an empty array if we are the
// root. This function is safe to be called from other actors.
func (s *state) coords() types.Coordinates {
	var coords types.Coordinates
	phony.Block(s, func() {
		coords = s._coords()
	})
	return coords
}

// _coords returns our tree coordinates, or an empty array if we are the
// root.
func (s *state) _coords() types.Coordinates {
	if ann := s._rootAnnouncement(); ann != nil {
		return ann.Coords()
	}
	return types.Coordinates{}
}

// _becomeRoot removes our current parent, effectively making us a root
// node. It then kicks off tree maintenance, which will result in a tree
// announcement being sent to our peers.
func (s *state) _becomeRoot() {
	if s._parent == nil {
		return
	}
	s._setParent(nil)
	s._maintainTree()
}

// sendTreeAnnouncementToPeer signs and sends the given root announcement
// to a given peer.
func (s *state) sendTreeAnnouncementToPeer(ann *rootAnnouncementWithTime, p *peer) {
	p.proto.push(ann.forPeer(p))
}

// _sendTreeAnnouncements signs and sends the current root announcement to
// all of our active peers.
func (s *state) _sendTreeAnnouncements() {
	ann := s._rootAnnouncement()
	for _, p := range s._peers {
		if p == nil || p.port == 0 || !p.started.Load() {
			continue
		}
		s.sendTreeAnnouncementToPeer(ann, p)
	}

	s.r.Act(nil, func() {
		coords := []uint64{}
		for _, val := range ann.Coords() {
			coords = append(coords, uint64(val))
		}

		var announcementTime int64
		if ann.RootPublicKey == s.r.public {
			announcementTime = time.Now().UnixNano()
		} else {
			announcementTime = ann.receiveTime.UnixNano()
		}

		s.r._publish(events.TreeRootAnnUpdate{
			Root:     ann.RootPublicKey.String(),
			Sequence: uint64(ann.RootSequence),
			Time:     uint64(announcementTime),
			Coords:   coords,
		})
	})
}

type treeNextHopParams struct {
	destinationCoords types.Coordinates
	ourCoords         types.Coordinates
	fromPeer          *peer
	selfPeer          *peer
	lastAnnouncement  *rootAnnouncementWithTime
	peerAnnouncements *announcementTable
}

// _nextHopsTree returns the best next-hop candidate for a given frame. The
// "from" peer must be supplied in order to prevent routing loops. It is
// possible for this function to return nil if no next best-hop is available.
func (s *state) _nextHopsTree(from *peer, dest types.Coordinates) *peer {
	nextHopParams := treeNextHopParams{
		dest,
		s._coords(),
		from,
		s.r.local,
		s._rootAnnouncement(),
		&s._announcements,
	}

	return getNextHopTree(nextHopParams)
}

func getNextHopTree(params treeNextHopParams) *peer {
	// If it's loopback then don't bother doing anything else.
	if params.destinationCoords.EqualTo(params.ourCoords) {
		return params.selfPeer
	}

	// Work out how close our own coordinates are to the destination
	// message. This is important because we'll only forward a frame
	// to a peer that takes the message closer to the destination than
	// we are.
	ourDist := int64(params.ourCoords.DistanceTo(params.destinationCoords))
	if ourDist == 0 {
		// It's impossible to get closer so there's a pretty good
		// chance at this point that the traffic is destined for us.
		// Pass it up to the router.
		return params.selfPeer
	}

	// Now work out which of our peers takes the message closer.
	var bestPeer *peer
	bestDist := ourDist
	bestType := math.MaxUint16
	bestOrdering := uint64(math.MaxUint64)
	ourRoot := params.lastAnnouncement
	for p, ann := range *params.peerAnnouncements {
		switch {
		case !p.started.Load():
			continue // ignore peers that have stopped
		case ann == nil:
			continue // ignore peers that haven't sent us announcements
		case p == params.fromPeer:
			continue // don't route back where the packet came from
		case !ourRoot.Root.EqualTo(&ann.Root):
			continue // ignore peers that are following a different root or seq
		}

		// Look up the coordinates of the peer, and the distance
		// across the tree to those coordinates.
		peerCoords := ann.PeerCoords()
		peerDist := int64(peerCoords.DistanceTo(params.destinationCoords))
		peerType := int(p.peertype)
		if isBetterNextHopCandidate(
			peerType, peerDist, ann.receiveOrder,
			bestType, bestDist, bestOrdering,
			bestPeer == p,
		) {
			bestPeer, bestDist, bestOrdering, bestType = p, peerDist, ann.receiveOrder, peerType
		}
	}

	return bestPeer
}

func isBetterNextHopCandidate(
	peerType int, peerDistance int64, peerOrder uint64,
	bestType int, bestDistance int64, bestOrder uint64,
	isAnotherLinkToBest bool,
) bool {
	betterCandidate := false

	switch {
	case peerDistance < bestDistance:
		// The peer is closer to the destination.
		betterCandidate = true
	case peerDistance > bestDistance:
		// The peer is further away from the destination.
	case isAnotherLinkToBest && peerType < bestType:
		// This is another peering to the same node but is
		// a faster connection medium.
		betterCandidate = true
	case isAnotherLinkToBest && peerType == bestType && peerOrder < bestOrder:
		// This is another peering to the same node but has
		// a lower path to the root as a last-resort tiebreak.
		betterCandidate = true
	}

	return betterCandidate
}

type TreeAnnouncementAction int64

const (
	DropFrame TreeAnnouncementAction = iota // Default value
	AcceptUpdate
	AcceptNewParent
	SelectNewParent
	SelectNewParentWithWait
	InformPeerOfStrongerRoot
)

// _handleTreeAnnouncement is called whenever a tree announcement is
// received from a direct peer. It stores the update and then works out
// if that update is good news or bad news.
func (s *state) _handleTreeAnnouncement(p *peer, f *types.Frame) error {
	// Unmarshal the frame and check that it is sane. The sanity checks
	// do things like ensure that all updates are signed, the first
	// signature is from the root, the last signature is from our direct
	// peer etc.
	var newUpdate types.SwitchAnnouncement
	if _, err := newUpdate.UnmarshalBinary(f.Payload); err != nil {
		return fmt.Errorf("update unmarshal failed: %w", err)
	}
	if err := newUpdate.SanityCheck(p.public); err != nil {
		return fmt.Errorf("update sanity checks failed: %w", err)
	}

	isFirstAnnouncement := false
	shouldSendBroadcast := false

	// If the peer is replaying an old sequence number to us then we
	// assume that they are up to no good.
	if ann := s._announcements[p]; ann != nil {
		if newUpdate.RootPublicKey == ann.RootPublicKey && newUpdate.RootSequence < ann.RootSequence {
			return fmt.Errorf("update replays old sequence number")
		}
	} else {
		isFirstAnnouncement = true
		shouldSendBroadcast = true

		for peer, ann := range s._announcements {
			if ann != nil {
				if peer.public.CompareTo(p.public) == 0 {
					// We already have an established connection with this peer so sending another
					// broadcast would be redundant.
					shouldSendBroadcast = false
					break
				}
			}
		}
	}

	// Get the key of our current root and then work out if the root
	// key in the new update is stronger, weaker or the same key.
	lastParentUpdate := s._rootAnnouncement()
	lastRootKey := s.r.public
	if lastParentUpdate != nil {
		lastRootKey = lastParentUpdate.RootPublicKey
	}
	rootDelta := newUpdate.RootPublicKey.CompareTo(lastRootKey)

	// Save the root announcement for the peer. If the update is not
	// obviously bad then it isn't safe to "skip" storing updates.
	s._ordering++
	s._announcements[p] = &rootAnnouncementWithTime{
		SwitchAnnouncement: newUpdate,
		receiveTime:        time.Now(),
		receiveOrder:       s._ordering,
	}

	// If we're currently waiting to re-parent then there is no
	// further action
	if !s._waiting {
		announcementAction := determineAnnouncementAction(p == s._parent,
			newUpdate.IsLoopOrChildOf(s.r.public), rootDelta,
			newUpdate.RootSequence, lastParentUpdate.RootSequence)

		switch announcementAction {
		case DropFrame:
			// Do nothing
		case AcceptUpdate:
			s._sendTreeAnnouncements()
		case AcceptNewParent:
			s._setParent(p)
			s._sendTreeAnnouncements()
		case SelectNewParent:
			if s._selectNewParent() {
				s._bootstrapSoon()
			}
		case SelectNewParentWithWait:
			s._waiting = true
			s._becomeRoot()
			// Start the 1 second timer to re-run parent selection.
			time.AfterFunc(time.Second, func() {
				s.Act(nil, func() {
					s._waiting = false
					if s._selectNewParent() {
						s._bootstrapSoon()
					}
				})
			})
		case InformPeerOfStrongerRoot:
			if !isFirstAnnouncement {
				s.sendTreeAnnouncementToPeer(lastParentUpdate, p)
			}
		}
	}

	if shouldSendBroadcast {
		if broadcast, err := s._createBroadcastFrame(); err == nil {
			p.send(broadcast)
		}
	}

	return nil
}

// determineAnnouncementAction performs the algorithm used to decide how to react
// when a new tree announcement is received.
func determineAnnouncementAction(senderIsParent bool, updateContainsLoop bool,
	rootDelta int, newRootSequence types.Varu64, lastRootSequence types.Varu64) TreeAnnouncementAction {
	action := DropFrame
	if senderIsParent { // The update came from our current parent.
		switch {
		case updateContainsLoop:
			// The update seems to contain our own key already, so it
			// would appear that our chosen parent has suddenly decided
			// to start replaying our own updates back to us. This is
			// bad news.
			action = SelectNewParentWithWait
		case rootDelta < 0:
			// The update contains a weaker root key, which is also bad
			// news.
			action = SelectNewParentWithWait
		case rootDelta == 0 && newRootSequence == lastRootSequence:
			// The update contains the same root key, but the sequence
			// number is being replayed. This usually happens when the
			// parent has chosen a new parent and is re-signing the last
			// update to notify their peers of their new coordinates.
			// In this case, we consider this also to be bad news. We
			// will switch to being the root, which notifies our peers
			// of the bad news (since the update will be coming from a
			// weaker key) and then we start a 1 second timer, after
			// which we will re-run the parent selection. During that 1
			// second period, we will not act on root updates apart from
			// saving them.
			action = SelectNewParentWithWait
		case rootDelta > 0:
			// The root update contains a stronger key than before.
			// Since this node is already our parent, we can just send out
			// the update as normal.
			action = AcceptUpdate
		case rootDelta == 0 && newRootSequence > lastRootSequence:
			// The root update contains the same key as before but it has
			// a new sequence number, so the parent is repeating a new
			// update to us. We will repeat that update to our peers.
			action = AcceptUpdate
		}
	} else { // Update came from another peer
		switch {
		case updateContainsLoop:
			// The update seems to be signed to us already. This happens
			// because one of our peers has chosen us as their parent, but
			// they still have to send an update back to us so that we know
			// their coordinates. In this case, we will not do anything more
			// with the update since it would create a loop otherwise.
			action = DropFrame
		case rootDelta > 0:
			// The update seems to contain a stronger root than our existing
			// root. In that case, we will switch to this node as our parent
			// and then send out tree announcements to our peers, notifying
			// them of the change.
			action = AcceptNewParent
		case rootDelta < 0:
			// The update seems to contain a weaker root key than our existing
			// root. In this case the best thing to do is to send an update
			// back to this specific peer containing our stronger key in the
			// hope that they will accept the update and re-parent.
			action = InformPeerOfStrongerRoot
		default:
			// The update contains the same root key so we will check if it
			// still makes sense to keep our current parent. We will reparent
			// if not, sending out a new SNEK bootstrap into the network.
			action = SelectNewParent
		}
	}

	return action
}

// _selectNewParent will examine the root updates from all of our peers
// and decide if we should re-parent. If a new peer is selected, this
// function will return true. If no change is made, or we become the root
// as a result, this function will return false.
func (s *state) _selectNewParent() bool {
	// Start with our current root key as the strongest candidate. If we
	// don't have any peers that also have this root update then this will
	// cause us to fail parent selection, marking ourselves as the root.
	root := s._rootAnnouncement()
	bestRoot := root.Root

	// If our own key happens to be stronger than our current root for some
	// reason then we will just compare against our own key instead.
	if bestRoot.RootPublicKey.CompareTo(s.r.public) < 0 {
		bestRoot = types.Root{
			RootPublicKey: s.r.public,
			RootSequence:  0,
		}
	}
	bestOrder := uint64(math.MaxUint64)
	var bestPeer *peer

	// Iterate through all of the announcements received from our peers.
	// This will exclude any peers that haven't sent us updates yet.
	for peer, ann := range s._announcements {
		if !peer.started.Load() {
			// The peer has been stopped for some reason, possibly due to a
			// timeout or other protocol handling error.
			continue
		}

		if ann != nil {
			if isBetterParentCandidate(*ann, bestRoot, bestOrder, ann.IsLoopOrChildOf(s.r.public)) {
				bestRoot = ann.Root
				bestPeer = peer
				bestOrder = ann.receiveOrder
			}
		}
	}

	// If we found a suitable candidate then we should see if a change needs
	// to be made.
	if bestPeer != nil {
		if bestPeer != s._parent {
			// The chosen candidate is different to our current parent, so we
			// will update to our new parent and then send tree announcements
			// to our peers to notify them of the change.
			s._setParent(bestPeer)
			s._sendTreeAnnouncements()
			return true
		}
		// The chosen candidate is the same as our current parent, so there is
		// nothing to do.
		return false
	}

	// No suitable other peer was found, so we'll just become the root and wait
	// for one of our peers corrects us with future updates.
	s._becomeRoot()
	return false
}

func isBetterParentCandidate(ann rootAnnouncementWithTime, bestRoot types.Root,
	bestOrder uint64, containsLoop bool) bool {
	isBetterCandidate := false

	if time.Since(ann.receiveTime) >= announcementTimeout {
		// If the announcement has expired then don't consider this peer
		// as a possible candidate.
		return false
	}

	// Work out if the parent's announcement contains a stronger root
	// key than our current best candidate.
	keyDelta := ann.RootPublicKey.CompareTo(bestRoot.RootPublicKey)
	switch {
	case containsLoop:
		// The announcement from this peer contains our own public key in
		// the signatures, which implies they are a child of ours in the
		// tree. We therefore can't use this peer as a parent as this would
		// create a loop in the tree.
	case keyDelta > 0:
		// The peer has a stronger root key, so they are a better candidate.
		isBetterCandidate = true
	case keyDelta < 0:
		// The peer has a weaker root key than our current best candidate,
		// so ignore this peer.
	case ann.RootSequence > bestRoot.RootSequence:
		// The peer has the same root key as our current candidate but the
		// sequence number is higher, so they have sent us a newer tree
		// announcement. They are a better candidate as a result.
		isBetterCandidate = true
	case ann.RootSequence < bestRoot.RootSequence:
		// The peer has the same root key as our current candidate but a
		// worse sequence number, so their announcement is out of date.
	case ann.receiveOrder < bestOrder:
		// The peer has the same root key and update sequence number as our
		// current best candidate, but the update from this peer was received
		// first. This condition is a tie-break that helps us to pick a parent
		// which will have the lowest latency path to the root, all else equal.
		isBetterCandidate = true
	}

	return isBetterCandidate
}
