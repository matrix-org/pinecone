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
)

// announcementInterval is the frequency at which this
// node will send root announcements to other peers.
const announcementInterval = time.Minute * 15

// announcementTimeout is the amount of time that must
// pass without receiving a root announcement before we
// will assume that the peer is dead.
const announcementTimeout = announcementInterval * 2

func (r *Router) handleAnnouncement(peer *Peer, rx *types.Frame) {
	var new types.SwitchAnnouncement
	if _, err := new.UnmarshalBinary(rx.Payload); err != nil {
		r.log.Println("Error unmarshalling announcement:", err)
		return
	}
	if err := r.tree.Update(peer, new, false); err != nil {
		r.log.Println("Error handling announcement on port", peer.port, ":", err)
	}
}

type rootAnnouncementWithTime struct {
	types.SwitchAnnouncement
	at time.Time
}

type spanningTree struct {
	r           *Router         //
	context     context.Context //
	rootReset   chan struct{}   //
	updateMutex sync.Mutex      //
	parent      atomic.Value    // types.SwitchPortID
	callback    func(parent types.SwitchPortID, coords types.SwitchPorts)
}

func newSpanningTree(r *Router, f func(parent types.SwitchPortID, coords types.SwitchPorts)) *spanningTree {
	t := &spanningTree{
		r:         r,
		context:   r.context,
		rootReset: make(chan struct{}),
		callback:  f,
	}
	t.becomeRoot()
	go t.workerForRoot()
	go t.workerForAnnouncements()
	return t
}

func (t *spanningTree) Coords() types.SwitchPorts {
	parent, ok := t.parent.Load().(types.SwitchPortID)
	if !ok {
		return types.SwitchPorts{}
	}
	ann := t.r.ports[parent].lastAnnouncement()
	if ann == nil {
		return types.SwitchPorts{}
	}
	return ann.Coords()
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
	if parent := t.parent.Load().(types.SwitchPortID); parent == port {
		t.selectNewParent()
	}
}

func (t *spanningTree) selectNewParent() {
	t.updateMutex.Lock()
	bestDist := math.MaxInt32
	bestKey := t.r.public
	var bestTime time.Time
	var bestPort types.SwitchPortID
	var bestAnn *rootAnnouncementWithTime
	var bestSeq types.Varu64
	portsToCheck := map[*Peer]*rootAnnouncementWithTime{}
	for _, p := range t.r.activePorts() {
		if p.child.Load() {
			continue
		}
		ann := p.lastAnnouncement()
		if ann == nil {
			continue
		}
		portsToCheck[p] = ann
	}
	checkWithCondition := func(f func(ann *rootAnnouncementWithTime, hops int) bool) {
		for p, ann := range portsToCheck {
			hops := len(ann.Signatures)
			if f(ann, hops) {
				bestKey = ann.RootPublicKey
				bestDist = hops
				bestPort = p.port
				bestTime = ann.at
				bestSeq = ann.Sequence
				bestAnn = ann
			}
		}
	}
	checkWithCondition(func(ann *rootAnnouncementWithTime, hops int) bool {
		return ann.RootPublicKey.CompareTo(bestKey) > 0
	})
	checkWithCondition(func(ann *rootAnnouncementWithTime, hops int) bool {
		return ann.RootPublicKey.CompareTo(bestKey) == 0 && ann.Sequence > bestSeq
	})
	checkWithCondition(func(ann *rootAnnouncementWithTime, hops int) bool {
		return ann.RootPublicKey.CompareTo(bestKey) == 0 && ann.Sequence == bestSeq && hops < bestDist
	})
	checkWithCondition(func(ann *rootAnnouncementWithTime, hops int) bool {
		return ann.RootPublicKey.CompareTo(bestKey) == 0 && ann.Sequence == bestSeq && hops == bestDist && ann.at.Before(bestTime)
	})
	t.updateMutex.Unlock()
	if bestAnn != nil {
		if err := t.Update(t.r.ports[bestPort], bestAnn.SwitchAnnouncement, true); err != nil {
			t.r.log.Println("t.Update: %w", err)
		}
	} else {
		t.becomeRoot()
	}
}

func (t *spanningTree) advertise() {
	for _, p := range t.r.startedPorts() {
		go func(p *Peer) {
			select {
			case <-p.context.Done():
			case <-time.After(announcementTimeout):
			case p.announce <- struct{}{}:
			}
		}(p)
	}
}

func (t *spanningTree) becomeRoot() {
	t.parent.Store(types.SwitchPortID(0))
	newCoords := types.SwitchPorts{}
	if !t.Coords().EqualTo(newCoords) {
		go t.callback(0, types.SwitchPorts{})
		defer t.r.snake.rootNodeChanged(t.r.public)
	}
	t.advertise()
}

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

func (t *spanningTree) workerForRoot() {
	for {
		select {
		case <-t.context.Done():
			return

		case <-t.rootReset:

		case <-time.After(announcementTimeout):
			if !t.IsRoot() {
				t.selectNewParent()
			}
		}
	}
}

func (t *spanningTree) IsRoot() bool {
	root := t.Root()
	return root.RootPublicKey.EqualTo(t.r.public) || time.Since(root.at) >= announcementTimeout
}

func (t *spanningTree) Root() *rootAnnouncementWithTime {
	root := t.r.ports[t.Parent()].lastAnnouncement()
	if root == nil {
		return &rootAnnouncementWithTime{
			at: time.Now(),
			SwitchAnnouncement: types.SwitchAnnouncement{
				RootPublicKey: t.r.public,
				Sequence:      types.Varu64(time.Now().UnixNano()),
			},
		}
	}
	return &rootAnnouncementWithTime{
		at: time.Now(),
		SwitchAnnouncement: types.SwitchAnnouncement{ // return a copy
			RootPublicKey: root.RootPublicKey,
			Sequence:      root.Sequence,
			Signatures:    append([]types.SignatureWithHop{}, root.Signatures...),
		},
	}
}

func (t *spanningTree) Parent() types.SwitchPortID {
	if parent, ok := t.parent.Load().(types.SwitchPortID); ok {
		return parent
	}
	return 0
}

func (t *spanningTree) Update(p *Peer, newUpdate types.SwitchAnnouncement, updateParent bool) error {
	sigs := make(map[string]struct{})
	for index, sig := range newUpdate.Signatures {
		if index == 0 && sig.PublicKey != newUpdate.RootPublicKey {
			// The first signature in the announcement must be from the
			// key that claims to be the root.
			return fmt.Errorf("rejecting update (first signature must be from root)")
		}
		if sig.Hop == 0 {
			// None of the hops in the update should have a port number of 0
			// as this would imply that another node has sent their router
			// port, which is impossible. We'll therefore reject any update
			// that tries to do that.
			return fmt.Errorf("rejecting update (invalid 0 hop)")
		}
		if index == len(newUpdate.Signatures)-1 && p.PublicKey() != sig.PublicKey {
			// The last signature in the announcement must be from the
			// direct peer. If it isn't then it sounds like someone is
			// trying to replay someone else's announcement to us.
			return fmt.Errorf("rejecting update (last signature must be from peer)")
		}
		pk := hex.EncodeToString(sig.PublicKey[:])
		if _, ok := sigs[pk]; ok {
			// One of the signatures has appeared in the update more than
			// once, which would suggest that there's a loop somewhere.
			return fmt.Errorf("rejecting update (detected routing loop)")
		}
		sigs[pk] = struct{}{}
	}

	lastPortUpdate := p.lastAnnouncement()
	if lastPortUpdate != nil && lastPortUpdate.RootPublicKey == newUpdate.RootPublicKey {
		if newUpdate.Sequence < lastPortUpdate.Sequence {
			// The update has a lower sequence number than our last update
			// on this port from this root. This shouldn't happen, but if it
			// does, then just drop the update.
			return nil
		}
	}

	lastParentUpdate := t.Root()
	if err := p.updateAnnouncement(&newUpdate); err != nil {
		return fmt.Errorf("p.updateAnnouncement: %w", err)
	}

	if lastPortUpdate != nil {
		// Check if anything strange has happened to our parent that might
		// indicate a problem with their parent, like a sudden lengthening
		// of the path or a root key change altogether.
		if parent := t.parent.Load().(types.SwitchPortID); p.port == parent {
			switch {
			case newUpdate.RootPublicKey.CompareTo(lastPortUpdate.RootPublicKey) < 0: // the root key got weaker
				t.selectNewParent()
				return nil

			case len(newUpdate.Signatures)-len(lastPortUpdate.Signatures) > 0: // the path got longer
				t.selectNewParent()
				return nil
			}
		}
	}

	if p.child.Load() {
		return nil
	}

	t.updateMutex.Lock()
	defer t.updateMutex.Unlock()

	if !updateParent {
		keyDeltaSinceLastParentUpdate := newUpdate.RootPublicKey.CompareTo(lastParentUpdate.RootPublicKey)

		switch {
		case time.Since(lastParentUpdate.at) > announcementTimeout:
			// We haven't seen a suitable root update recently so we'll accept
			// this one instead
			updateParent = true

		case keyDeltaSinceLastParentUpdate < 0:
			// The root key is weaker than our existing root so ignore it.
			return nil

		case keyDeltaSinceLastParentUpdate == 0:
			// The update is from the same root node, let's see if it matches
			// any other useful conditions
			switch {
			case newUpdate.Sequence < lastParentUpdate.Sequence:
				// This is a replay of an earlier update, therefore we should
				// ignore it, even if it came from our parent node
				return nil

			case len(newUpdate.Signatures) < len(lastParentUpdate.Signatures):
				// The path to the root is shorter than our last update, so
				// we'll accept it
				updateParent = true
			}

		case keyDeltaSinceLastParentUpdate > 0:
			// The root key is stronger than our existing root, therefore we'll
			// accept it anyway, since we always want to converge on the
			// strongest root key
			updateParent = true
		}
	}

	if parent, ok := t.parent.Load().(types.SwitchPortID); !ok || p.port == parent || updateParent {
		oldcoords, oldroot := lastParentUpdate.Coords(), lastParentUpdate.RootPublicKey
		t.parent.Store(p.port)
		newcoords, newroot := t.Coords(), t.Root().RootPublicKey

		if t.callback != nil && !oldcoords.EqualTo(newcoords) {
			go t.callback(p.port, newcoords)
		}

		if oldroot != newroot {
			defer t.r.snake.rootNodeChanged(newroot)
		}

		t.advertise()
		t.rootReset <- struct{}{}
	}

	return nil
}
