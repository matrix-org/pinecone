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
	"math"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

// announcementInterval is the frequency at which this
// node will send root announcements to other peers.
const announcementInterval = time.Minute * 15

// announcementTimeout is the amount of time that must
// pass without receiving a root announcement before we
// will assume that the peer is dead.
const announcementTimeout = announcementInterval * 2

// immediately is used for selectNewParentAndAdvertiseIn.
const immediately = time.Duration(0)

func (r *Router) handleAnnouncement(peer *Peer, rx *types.Frame) error {
	var newUpdate types.SwitchAnnouncement
	if _, err := newUpdate.UnmarshalBinary(rx.Payload); err != nil {
		return fmt.Errorf("failed to unmarshal root announcement: %w", err)
	}
	if len(newUpdate.Signatures) == 0 {
		// An update that has no signatures whatsoever is invalid. Drop it.
		return fmt.Errorf("root announcement is not signed")
	}
	sigs := make(map[string]struct{})
	for index, sig := range newUpdate.Signatures {
		if index == 0 && sig.PublicKey != newUpdate.RootPublicKey {
			// The first signature in the announcement must be from the
			// key that claims to be the root node.
			return fmt.Errorf("root announcement first signature is not from the root node")
		}
		if sig.Hop == 0 {
			// None of the hops in the update should have a port number of 0
			// as this would imply that another node has sent their router
			// port, which is impossible. We'll therefore reject any update
			// that tries to do that.
			return fmt.Errorf("root announcement contains an invalid port number")
		}
		if index == len(newUpdate.Signatures)-1 && peer.PublicKey() != sig.PublicKey {
			// The last signature in the announcement must be from the
			// direct peer. If it isn't then it sounds like someone is
			// trying to replay someone else's announcement to us.
			return fmt.Errorf("root announcement last signature is not from the direct peer")
		}
		pk := hex.EncodeToString(sig.PublicKey[:])
		if _, ok := sigs[pk]; ok {
			// One of the signatures has appeared in the update more than
			// once, which would suggest that there's a loop somewhere.
			return fmt.Errorf("root announcement contains a routing loop")
		}
		sigs[pk] = struct{}{}
	}

	// Update the announcement for this peer and work out their
	// new coordinates.
	if err := peer.updateAnnouncement(&newUpdate); err != nil {
		return fmt.Errorf("peer.updateAnnouncement: %w", err)
	}

	// Now check if the announcement compels us to update our chosen
	// parent in the tree for some reason, i.e. if the root key is
	// stronger, our current root has fallen silent or similar.
	r.tree.UpdateParentIfNeeded(peer, newUpdate)
	return nil
}

type rootAnnouncementWithTime struct {
	types.SwitchAnnouncement
	receiveTime  time.Time // when did we receive the update?
	receiveOrder uint64    // the relative order that the update was received
}

func (a *rootAnnouncementWithTime) ForPeer(p *Peer) *types.Frame {
	if p.port == 0 {
		return nil
	}
	announcement := a.SwitchAnnouncement
	announcement.Signatures = append([]types.SignatureWithHop{}, a.Signatures...)
	for _, sig := range announcement.Signatures {
		if p.r.public.EqualTo(sig.PublicKey) {
			// For some reason the announcement that we want to send already
			// includes our signature. This shouldn't really happen but if we
			// did send it, other nodes would end up ignoring the announcement
			// anyway since it would appear to be a routing loop.
			return nil
		}
	}
	// Sign the announcement.
	if err := announcement.Sign(p.r.private[:], p.port); err != nil {
		p.r.log.Println("Failed to sign switch announcement:", err)
		return nil
	}
	payload := bufPool.Get().(*[types.MaxFrameSize]byte)
	defer bufPool.Put(payload)
	n, err := announcement.MarshalBinary(payload[:])
	if err != nil {
		p.r.log.Println("Failed to marshal switch announcement:", err)
		return nil
	}
	frame := types.GetFrame()
	frame.Version = types.Version0
	frame.Type = types.TypeSTP
	frame.Destination = types.SwitchPorts{}
	frame.Payload = append(frame.Payload[:0], payload[:n]...)
	return frame
}

type spanningTree struct {
	r         *Router
	context   context.Context    // cancelled when the node shuts down
	rootReset chan struct{}      // a struct{} is sent into this channel every time we get a valid root update from our parent
	mutex     *sync.Mutex        // used to de-race updates to the spanning tree
	parent    types.SwitchPortID // our chosen parent, or 0 if we are the root
	reparent  *time.Timer        // used by selectNewParentAndAdvertiseIn to de-dupe/delay calls to selectNewParentAndAdvertise
	sequence  atomic.Uint64      // the sequence number we send into the network if we're the root (unused if we aren't the root)
	ordering  uint64             // a global counter used to work out the order that our peers sent us updates, more accurate than measuring time

	// callback is called when the parent or the coordinates change. API
	// users can hook into this to be notified of these events.
	callback func(parent types.SwitchPortID, coords types.SwitchPorts)
}

func newSpanningTree(r *Router, f func(parent types.SwitchPortID, coords types.SwitchPorts)) *spanningTree {
	t := &spanningTree{
		r:         r,
		context:   r.context,
		rootReset: make(chan struct{}),
		mutex:     &sync.Mutex{},
		callback:  f,
	}

	// Create a new timer for selectNewParentAndAdvertiseIn. The timer is used
	// to ensure that we don't make lots of duplicate calls to selectNewParentAndAdvertise,
	// and that if we're already delaying a call, it'll be interrupted and called once
	// rather than twice.
	t.reparent = time.AfterFunc(time.Second, t.selectNewParentAndAdvertise)
	t.reparent.Stop()

	// All nodes think of themselves as the root node when we first come online. Then
	// we'll send out root updates to our peers and they will send us theirs, and we'll
	// keep switching to stronger parents until we settle on a common root.
	t.becomeRoot()

	// Start the worker goroutines.
	go t.workerForRoot()
	go t.workerForAnnouncements()
	return t
}

// Coords returns the coordinates of the current node. If we think we are
// the root node then this will return []. Otherwise it will return the
// coordinates based on the last root announcement of our chosen parent.
func (t *spanningTree) Coords() types.SwitchPorts {
	parent := t.Parent()
	if parent == 0 {
		// We think we're the root node, so our coordinates are [].
		return types.SwitchPorts{}
	}
	ann := t.r.ports[parent].lastAnnouncement()
	if ann == nil {
		// This shouldn't really happen but the main reason that it could
		// happen is that our chosen parent is our only peer and they haven't
		// sent us a root update in a while. We should pick this up by the
		// annexpired peer timer and disconnect that peer, but it's possible
		// we might hit this in the meantime.
		return types.SwitchPorts{}
	}
	return ann.Coords()
}

// Ancestors returns the public keys of all of our ancestors — that is, nodes
// between us and the root node. It also returns the port number of our chosen
// parent.
func (t *spanningTree) Ancestors() ([]types.PublicKey, types.SwitchPortID) {
	root, port := t.Root(), t.Parent()
	if port == 0 {
		// If we believe we are the root node then we don't have any
		// ancestors.
		return nil, 0
	}
	ancestors := make([]types.PublicKey, 0, len(root.Signatures))
	for _, sig := range root.Signatures {
		ancestors = append(ancestors, sig.PublicKey)
	}
	return ancestors, port
}

// portWasDisconnected is called when one of our peers disconnects.
func (t *spanningTree) portWasDisconnected(port types.SwitchPortID) {
	if t.r.PeerCount(-1) == 0 {
		// There are no more peers connected, so we'll reset to believing
		// we are the root node of our new island network.
		t.becomeRoot()
		return
	}
	if t.Parent() == port {
		// Our chosen parent node disconnected, so we need to pick a new
		// one straight away.
		t.selectNewParentAndAdvertiseIn(immediately)
	}
}

// selectNewParentAndAdvertiseIn schedules a parent re-selection. If there
// is already an update scheduled, the previous call will be rescheduled and
// interrupted, so that we deduplicate calls.
func (t *spanningTree) selectNewParentAndAdvertiseIn(d time.Duration) {
	t.reparent.Reset(d)
}

// selectNewParentAndAdvertise works out if there's a better parent and,
// if so, switches to it. If there is no suitable parent then we will
// become a root node automatically. Avoid calling this directly - use
// selectNewParentAndAdvertiseIn instead so that it doesn't get called
// more times than necessary, since this can result in sending announcements
// to peers.
func (t *spanningTree) selectNewParentAndAdvertise() {
	// Get information about our current root so that we can compare against
	// it later.
	lastUpdate := t.Root()

	// Start with some sensible values from our last update. The goal of the
	// loop through our peers' announcements is to find values that are better
	// than these, in order to find a suitable parent.
	bestKey := lastUpdate.RootPublicKey
	bestSeq := lastUpdate.Sequence
	bestOrder := uint64(math.MaxUint64)
	var bestPort types.SwitchPortID
	var bestAnn *rootAnnouncementWithTime

	// Take out a lock on the spanning tree so that nothing else happens while
	// we are busy selecting a new parent.
	t.mutex.Lock()
	lastParent := t.parent

	// The peersPool is used to reduce allocations since we're already allocating
	// lots of these in the next-hops path.
	active := t.r.activePorts()
	defer peersPool.Put(active) // nolint:staticcheck

	// Look through all of our active peers.
	for _, p := range active {
		ann := p.lastAnnouncement()
		if ann == nil {
			// It shouldn't be possible for an active peer to have no announcement
			// but in the interest of avoiding panics, we'll skip it anyway.
			continue
		}
		accept := func() {
			// This function is called when a better candidate is found.
			bestKey = ann.RootPublicKey
			bestPort = p.port
			bestOrder = ann.receiveOrder
			bestSeq = ann.Sequence
			bestAnn = ann
		}
		keyDelta := ann.RootPublicKey.CompareTo(bestKey)
		switch {
		case ann.IsLoopOrChildOf(p.r.public):
			// Ignore any children that is a child of ours in the tree or
			// contains a routing loop.
		case keyDelta > 0:
			// This peer has sent us a root announcement with a root key that is
			// stronger than the previous parent, so it's a better candidate.
			accept()
		case keyDelta < 0:
			// This peer has sent us a root announcement with a weaker root key
			// so it isn't a suitable candidate. They will probably update their
			// root when we send our next announcement to them anyway.
		case ann.Sequence > bestSeq:
			// The update from this peer has an equal root key to the last update
			// from the previous parent but the sequence number is higher, so it's
			// a more recent update and is therefore a better candidate.
			accept()
		case ann.Sequence < bestSeq:
			// The update from this peer has a lower sequence number than the
			// last update we received so it is out of date.
		case ann.receiveOrder < bestOrder:
			// At this point we've found another update that is from the same root
			// and with the same sequence number, so we should pick the peer from
			// which the update arrived first – this approximates to something like
			// the shortest latency path to the root.
			accept()
		}
	}

	// Check if we have a suitable candidate — bestAnn will only be set if
	// we found a suitable parent.
	if bestAnn != nil {
		// Update to the new parent and then release the lock on the spanning
		// tree.
		t.parent = bestPort
		t.mutex.Unlock()

		// Work out if either our root, our parent or the sequence number changed.
		// This determines what we should do next.
		newCoords := bestAnn.Coords()
		parentChanged := lastParent != bestPort
		coordsChanged := !lastUpdate.Coords().EqualTo(bestAnn.Coords())
		rootChanged := bestAnn.RootPublicKey != lastUpdate.RootPublicKey
		sequenceChanged := lastUpdate.RootPublicKey == bestKey && bestSeq > lastUpdate.Sequence

		// If either the root or the coordinates changed, notify whatever function
		// is on the other end of the callback about the change.
		if rootChanged || coordsChanged {
			t.callback(bestPort, newCoords)
		}

		switch {
		case rootChanged:
			// The network root changed, so we should notify SNEK about this change.
			// Any SNEK path built using the old root node will be torn down as a
			// result, as SNEK is sensitive to the root node being the same across
			// the entire network.
			t.r.snake.rootNodeChanged(bestAnn.RootPublicKey)
			fallthrough

		case parentChanged, sequenceChanged:
			// The chosen parent has changed, or we've got an update with a new
			// sequence number. We should notify our direct peers of this change.
			t.advertise()
		}
		return
	} else {
		// We didn't find a more suitable root node, so we'll become the root.
		// This will, in turn, notify our peers because we will send out a root
		// announcement with ourselves as the root key. Our peers may interpret
		// this as bad news and re-parent if needed.
		t.mutex.Unlock()
		t.becomeRoot()
	}
}

// advertise relays a root announcement to our peers. If we are the
// root then we will send an announcement with our own key, otherwise we
// will repeat the announcement from our parent.
func (t *spanningTree) advertise() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Get the last announcement from our chosen parent.
	ann := t.r.ports[t.parent].lastAnnouncement()

	if t.parent == 0 || ann == nil {
		// At this point we believe we're the root node, so we will fabricate
		// a root update that advertises that fact to our peers. Note that in
		// this specific case, the Sequence number is increased.
		ann = &rootAnnouncementWithTime{
			receiveTime: time.Now(),
			SwitchAnnouncement: types.SwitchAnnouncement{
				RootPublicKey: t.r.public,
				Sequence:      types.Varu64(t.sequence.Inc()),
			},
		}
	}

	// Send the announcement to all of our connected peers, signing it
	// for each one in the process.
	started := t.r.startedPorts()
	defer peersPool.Put(started) // nolint:staticcheck
	for _, p := range started {
		p.protoOut.push(ann.ForPeer(p))
	}
}

// becomeRoot instructs this node to become a root node and to
// advertise that fact to our direct peers.
func (t *spanningTree) becomeRoot() {
	// Store our old and new coordinates so we can tell if they
	// changed - they likely will if we weren't the root before but
	// are now.
	root := t.Root()
	oldCoords := root.Coords()
	newCoords := types.SwitchPorts{}

	// Take the tree mutex and update the parent to 0. If we were
	// not previously the root,
	t.mutex.Lock()
	t.parent = 0
	t.mutex.Unlock()

	// Advertise the change to our direct peers.
	t.advertise()

	// If our coordinates don't match what they were before then
	// call the callback.
	if !oldCoords.EqualTo(newCoords) {
		go t.callback(0, newCoords)
	}

	// If we weren't the root before but we are now then notify
	// SNEK.
	if root.RootPublicKey != t.r.public {
		t.r.snake.rootNodeChanged(t.r.public)
	}
}

// workerForAnnouncements runs while the spanning tree is running with
// a timer that ticks each time we need to send a root announcement. Note
// that we only send a root announcement if we believe we're the root
// node, otherwise we will already be echoing announcements in response
// to announcements from another node.
func (t *spanningTree) workerForAnnouncements() {
	ticker := time.NewTicker(announcementInterval)
	defer ticker.Stop()
	for {
		select {
		case <-t.context.Done():
			return

		case <-ticker.C:
			if t.IsRoot() {
				t.advertise()
				t.rootReset <- struct{}{}
			}
		}
	}
}

// workerForRoot runs while the spanning tree is running. The loop
// is designed to reset our node back to being the root when we don't
// receive any valid updates from a parent for a while.
func (t *spanningTree) workerForRoot() {
	for {
		select {
		case <-t.context.Done():
			return

		case <-t.rootReset:
			// We received a valid root update from our parent so move
			// onto the next loop iteration, which in effect resets the
			// time.After call in the next case.

		case <-time.After(announcementTimeout):
			if !t.IsRoot() {
				t.becomeRoot()
			}
		}
	}
}

// IsRoot returns true if the node believes it is the root node or
// false otherwise.
func (t *spanningTree) IsRoot() bool {
	if t.Parent() == 0 {
		return true
	}
	root := t.Root()
	return root.RootPublicKey.EqualTo(t.r.public) || time.Since(root.receiveTime) >= announcementTimeout
}

// Root returns either the most recent root announcement from our parent, if it
// is recent enough, or our own root announcement if not.
func (t *spanningTree) Root() *rootAnnouncementWithTime {
	// If there's a suitable and recent enough update from our chosen parent
	// then we will return that update.
	if root := t.r.ports[t.Parent()].lastAnnouncement(); root != nil {
		return root
	}
	// If we don't have a suitable root update from a parent, we probably
	// believe we are the root, therefore we should return our own update.
	// Note that this does NOT increment the sequence number, otherwise
	// repeated calls to Root() will return updates with different sequence
	// numbers which can cause the tree to fall out of sync when new peers
	// connect.
	return &rootAnnouncementWithTime{
		receiveTime: time.Now(),
		SwitchAnnouncement: types.SwitchAnnouncement{
			RootPublicKey: t.r.public,
			Sequence:      types.Varu64(t.sequence.Load()),
		},
	}
}

// Parent returns the port number of our chosen parent. This will return 0
// if we believe we are the root node.
func (t *spanningTree) Parent() types.SwitchPortID {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.parent
}

// UpdateParentIfNeeded is called in response to every root update that the
// node receives. It works out if the root update should trigger us to either
// select a new parent or to become a root.
func (t *spanningTree) UpdateParentIfNeeded(p *Peer, newUpdate types.SwitchAnnouncement) {
	reparentIn, becomeRoot := time.Duration(-1), false
	lastParentUpdate, isParent := t.Root(), p.IsParent()
	keyDeltaSinceLastParentUpdate := newUpdate.RootPublicKey.CompareTo(lastParentUpdate.RootPublicKey)

	switch {
	case keyDeltaSinceLastParentUpdate > 0:
		// The peer has sent us a key that is stronger than our last update. We
		// want to reparent straight away because this is obviously a better
		// candidate.
		reparentIn = immediately

	case time.Since(lastParentUpdate.receiveTime) >= announcementTimeout:
		// It's been a while since we last heard from our parent so we should
		// really choose a new one if we can. This peer is probably disconnecting
		// around about now anyway so don't delay a reparent.
		reparentIn = immediately

	case isParent && keyDeltaSinceLastParentUpdate < 0:
		// Our parent sent us a weaker key than before — this implies that
		// something bad happened. This probably means that the peer's parents
		// changed and that something is happening in the tree, so in this case
		// we will wait for a short period of time to allow our other peers to
		// possibly respond in a similar way before selecting a new parent.
		reparentIn, becomeRoot = time.Second/2, true

	case isParent && keyDeltaSinceLastParentUpdate == 0 && newUpdate.Sequence <= lastParentUpdate.Sequence:
		// Our parent sent us a repeated sequence number for the same root -
		// this isn't good news either. As above, we will wait for a short period
		// of time to ensure that any bad news also propagates via our other peers
		// before we try to select a new parent.
		reparentIn, becomeRoot = time.Second/2, true
	}

	switch {
	case reparentIn >= 0:
		if becomeRoot && lastParentUpdate.RootPublicKey != t.r.public {
			// Whatever happened requires us to become the root temporarily before
			// selecting a new parent. Becoming the root is effectively propagating
			// bad news to our peers, since that will trigger them to select new
			// parents.
			t.becomeRoot()
		}
		// Then schedule a reparent after the specified amount of time.
		t.selectNewParentAndAdvertiseIn(reparentIn)

	case isParent && keyDeltaSinceLastParentUpdate == 0 && newUpdate.Sequence > lastParentUpdate.Sequence:
		// None of the bad news situations happened so we now just need to see if this
		// is an update that is worth repeating to our peers. In this case, it is worth
		// repeating because the following conditions are true:
		// 1. The update came from our chosen parent;
		// 2. The root key hasn't changed;
		// 3. The sequence number of the update has increased since the last one.
		t.advertise()
		t.rootReset <- struct{}{}
	}
}
