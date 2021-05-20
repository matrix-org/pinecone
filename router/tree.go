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
	"sync/atomic"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
)

// announcementThreshold is the amount of time that must
// pass before the node will accept a root announcement
// again from the same peer.
const announcementThreshold = announcementInterval / 2

// announcementInterval is the frequency at which this
// node will send root announcements to other peers.
const announcementInterval = time.Minute

// announcementTimeout is the amount of time that must
// pass without receiving a root announcement before we
// will assume that the peer is dead.
const announcementTimeout = announcementInterval * 4

func (r *Router) handleAnnouncement(peer *Peer, rx *types.Frame) {
	defer rx.Done()
	var new types.SwitchAnnouncement
	if _, err := new.UnmarshalBinary(rx.Payload); err != nil {
		r.log.Println("Error unmarshalling announcement:", err)
		return
	}
	if err := r.tree.Update(peer, new); err != nil {
		r.log.Println("Error handling announcement on port", peer.port, ":", err)
	}
}

// This tries to converge on a minimum spanning tree by optimising
// parent relationships for distance. All other metrics are ignored.

type rootAnnouncementWithTime struct {
	types.SwitchAnnouncement
	at time.Time
}

type spanningTree struct {
	r         *Router                   //
	context   context.Context           //
	advertise util.Dispatch             //
	root      *rootAnnouncementWithTime // last root announcement
	rootMutex sync.RWMutex              //
	rootReset util.Dispatch             //
	parent    atomic.Value              // types.SwitchPortID
	callback  func(parent types.SwitchPortID, coords types.SwitchPorts)
}

func newSpanningTree(r *Router, f func(parent types.SwitchPortID, coords types.SwitchPorts)) *spanningTree {
	t := &spanningTree{
		r:         r,
		context:   r.context,
		advertise: util.NewDispatch(),
		rootReset: util.NewDispatch(),
		callback:  f,
	}
	t.becomeRoot()
	t.advertise.Dispatch()
	go t.workerForRoot()
	go t.workerForAnnouncements()
	return t
}

func (t *spanningTree) Coords() types.SwitchPorts {
	t.rootMutex.RLock()
	defer t.rootMutex.RUnlock()
	coords := types.SwitchPorts{}
	if t.root == nil {
		return coords
	}
	for _, hop := range t.root.Signatures {
		coords = append(coords, types.SwitchPortID(hop.Hop))
	}
	return coords
}

func (t *spanningTree) Ancestors() ([]types.PublicKey, types.SwitchPortID) {
	root := t.Root()
	port, ok := t.parent.Load().(types.SwitchPortID)
	if !ok || port == 0 {
		return nil, 0
	}
	ancestors := make([]types.PublicKey, 0, 1+len(root.Signatures))
	ancestors = append(ancestors, root.RootPublicKey)
	if len(root.Signatures) == 0 {
		return ancestors, port
	}
	for _, sig := range root.Signatures[1:] {
		ancestors = append(ancestors, sig.PublicKey)
	}
	return ancestors, port
}

func (t *spanningTree) portWasDisconnected(port types.SwitchPortID) {
	if t.r.PeerCount(-1) == 0 {
		t.becomeRoot()
		return
	}
	if t.parent.Load() == port {
		t.selectParent()
	}
}

func (t *spanningTree) becomeRoot() {
	t.rootMutex.Lock()
	t.root = &rootAnnouncementWithTime{
		SwitchAnnouncement: types.SwitchAnnouncement{
			RootPublicKey: t.r.public,
			Sequence:      types.Varu64(time.Now().UnixNano()),
		},
		at: time.Now(),
	}
	t.rootMutex.Unlock()
	t.parent.Store(types.SwitchPortID(0))
	newCoords := types.SwitchPorts{}
	if !t.Coords().EqualTo(newCoords) {
		t.callback(0, types.SwitchPorts{})
	}
	t.rootReset.Dispatch()
}

func (t *spanningTree) selectParent() (types.SwitchPortID, bool) {
	oldParent := t.parent.Load()
	bestDist := int64(math.MaxInt64)
	var parent types.SwitchPortID
	for _, port := range t.r.activePorts() {
		if !port.SeenCommonRootRecently() {
			// The peer either hasn't sent us an announcement yet, or it's
			// sent us an invalid announcement with no signatures.
			continue
		}
		if parent != 0 && port.port == oldParent {
			break
		}
		ann := port.lastAnnouncement()
		if l := int64(len(ann.Signatures)); parent == 0 || l < bestDist {
			parent, bestDist = port.port, l
		}
	}
	t.parent.Store(parent)
	return parent, parent == oldParent
}

func (t *spanningTree) workerForAnnouncements() {
	advertise := func() {
		for _, p := range t.r.ports {
			if p.started.Load() {
				p.advertise.Dispatch()
			}
		}
	}
	for {
		select {
		case <-t.context.Done():
			return

		case <-time.After(announcementInterval):
			if t.IsRoot() {
				advertise()
			}

		case <-t.advertise:
			advertise()
		}
	}
}

func (t *spanningTree) workerForRoot() {
	for {
		select {
		case <-t.context.Done():
			return

		case <-t.rootReset:

		case <-time.After(announcementTimeout):
			if !t.IsRoot() {
				t.r.log.Println("Haven't heard from the root lately")
				t.becomeRoot()
			}
		}
	}
}

func (t *spanningTree) IsRoot() bool {
	t.rootMutex.RLock()
	defer t.rootMutex.RUnlock()
	return t.root.RootPublicKey.EqualTo(t.r.public) || time.Since(t.root.at) >= announcementTimeout
}

func (t *spanningTree) Root() types.SwitchAnnouncement {
	t.rootMutex.RLock()
	defer t.rootMutex.RUnlock()
	if t.root.RootPublicKey.EqualTo(t.r.public) || time.Since(t.root.at) > announcementTimeout {
		return types.SwitchAnnouncement{
			RootPublicKey: t.r.public,
			Sequence:      types.Varu64(time.Now().UnixNano()),
		}
	}
	return types.SwitchAnnouncement{ // return a copy
		RootPublicKey: t.root.RootPublicKey,
		Sequence:      t.root.Sequence,
		Signatures:    append([]types.SignatureWithHop{}, t.root.Signatures...),
	}
}

func (t *spanningTree) Parent() types.SwitchPortID {
	if parent, ok := t.parent.Load().(types.SwitchPortID); ok {
		return parent
	}
	return 0
}

func (t *spanningTree) Remove(p *Peer) {
	if t.Parent() == p.port {
		t.parent.Store(types.SwitchPortID(0))
	}
}

func (t *spanningTree) Update(p *Peer, a types.SwitchAnnouncement) error {
	old := p.announcement
	var timeSinceLastUpdate time.Duration
	if old != nil {
		timeSinceLastUpdate = time.Since(old.at)
	}

	// If the announcement is from the same root, or a weaker one, and
	// hasn't waited for the threshold to pass, then we'll stop here,
	// otherwise we will end up flooding downstream nodes.
	if old != nil && a.RootPublicKey.CompareTo(old.RootPublicKey) <= 0 {
		switch {
		case a.RootPublicKey.EqualTo(old.RootPublicKey) && a.Sequence <= old.Sequence:
			return fmt.Errorf("rejecting update (old sequence number %d <= %d)", a.Sequence, old.Sequence)
		case timeSinceLastUpdate > 0 && timeSinceLastUpdate < announcementThreshold:
			return fmt.Errorf("rejecting update (too soon after %s)", timeSinceLastUpdate)
		}
	}

	// Check that there are no routing loops in the update.
	sigs := make(map[string]struct{})
	isChild := false
	for _, sig := range a.Signatures {
		if sig.Hop == 0 {
			// None of the hops in the update should have a port number of 0
			// as this would imply that another node has sent their router
			// port, which is impossible. We'll therefore reject any update
			// that tries to do that.
			return fmt.Errorf("rejecting update (invalid 0 hop)")
		}
		if t.r.public.EqualTo(sig.PublicKey) {
			// It looks like the update contains our public key. This is not
			// strictly an error condition, since any of our children on the
			// spanning tree can send an update back to us with our own key,
			// but we don't act upon them because that would create loops.
			// Instead we'll just update the port announcement entry and stop.
			isChild = true
		}
		pk := hex.EncodeToString(sig.PublicKey[:])
		if _, ok := sigs[pk]; ok {
			// One of the signatures has appeared in the update more than
			// once, which would suggest that there's a loop somewhere.
			return fmt.Errorf("rejecting update (detected routing loop)")
		}
		sigs[pk] = struct{}{}
	}

	t.rootMutex.RLock()
	oldRoot, newRoot := t.root, t.root
	t.rootMutex.RUnlock()

	switch {
	case time.Since(oldRoot.at) > announcementTimeout:
		// We haven't had a root update from anyone else recently, so let's use
		// this instead.
		newRoot = &rootAnnouncementWithTime{a, time.Now()}

	case a.RootPublicKey.CompareTo(oldRoot.RootPublicKey) > 0:
		// If the advertisement contains a stronger key than the root, or the
		// announcement contains the root that we know about, update our stored
		// announcement.
		newRoot = &rootAnnouncementWithTime{a, time.Now()}

	case oldRoot.RootPublicKey.EqualTo(a.RootPublicKey) && a.Sequence > oldRoot.Sequence:
		// We'll only process the update from the same root if it's actually
		// a new update, e.g. the sequence number has increased, and the
		// signature count is equal to or shorter than the previous count.
		// This stops us from flapping coordinates so much.
		if !isChild && len(a.Signatures) <= len(oldRoot.Signatures) {
			newRoot = &rootAnnouncementWithTime{a, time.Now()}
		}

	default:
		//return fmt.Errorf("no conditions met")
	}

	// Store the announcement against the peer. This lets us ultimately
	// calculate what the coordinates of that peer are later.
	p.alive.Store(true)
	p.mutex.Lock()
	p.announcement = &rootAnnouncementWithTime{
		SwitchAnnouncement: a,
		at:                 time.Now(),
	}
	p.updateCoords(p.announcement)
	p.mutex.Unlock()

	t.rootReset.Dispatch()

	// If the root has changed then let's do something about it.
	if newRoot != oldRoot {
		t.rootMutex.Lock()
		t.root = newRoot
		t.rootMutex.Unlock()

		if parent, _ := t.selectParent(); p.port == parent {

		}
		t.advertise.Dispatch()
	}
	if newRoot.RootPublicKey != oldRoot.RootPublicKey {
		go t.r.snake.rootNodeChanged(newRoot.RootPublicKey)
	}

	return nil
}
