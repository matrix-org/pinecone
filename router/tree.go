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

func (r *Router) handleAnnouncement(peer *Peer, rx *types.Frame) {
	var newUpdate types.SwitchAnnouncement
	if _, err := newUpdate.UnmarshalBinary(rx.Payload); err != nil {
		r.log.Println("Error unmarshalling announcement:", err)
		return
	}
	sigs := make(map[string]struct{})
	for index, sig := range newUpdate.Signatures {
		if index == 0 && sig.PublicKey != newUpdate.RootPublicKey {
			// The first signature in the announcement must be from the
			// key that claims to be the root.
			return
		}
		if sig.Hop == 0 {
			// None of the hops in the update should have a port number of 0
			// as this would imply that another node has sent their router
			// port, which is impossible. We'll therefore reject any update
			// that tries to do that.
			return
		}
		if index == len(newUpdate.Signatures)-1 && peer.PublicKey() != sig.PublicKey {
			// The last signature in the announcement must be from the
			// direct peer. If it isn't then it sounds like someone is
			// trying to replay someone else's announcement to us.
			return
		}
		pk := hex.EncodeToString(sig.PublicKey[:])
		if _, ok := sigs[pk]; ok {
			// One of the signatures has appeared in the update more than
			// once, which would suggest that there's a loop somewhere.
			return
		}
		sigs[pk] = struct{}{}
	}

	lastPortUpdate := peer.lastAnnouncement()
	if lastPortUpdate != nil && lastPortUpdate.RootPublicKey == newUpdate.RootPublicKey {
		if newUpdate.Sequence < lastPortUpdate.Sequence {
			// The update is a replay of a previous announcement which doesn't
			// make sense
			peer.cancel()
			return
		}
	}

	if err := peer.updateAnnouncement(&newUpdate); err != nil {
		r.log.Println("Error updating announcement on port", peer.port, ":", err)
		return
	}

	if err := r.tree.Update(peer, newUpdate, false); err != nil {
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
	ann := t.r.ports[t.Parent()].lastAnnouncement()
	if ann == nil {
		return types.SwitchPorts{}
	}
	return ann.Coords()
}

func (t *spanningTree) Ancestors() ([]types.PublicKey, types.SwitchPortID) {
	root, port := t.Root(), t.Parent()
	if port == 0 {
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
	if t.Parent() == port {
		t.selectNewParentAndAdvertise()
	}
}

func (t *spanningTree) selectNewParentAndAdvertise() {
	lastParentUpdate := t.Root()
	lastParentPort := t.Parent()

	t.selectNewParent()

	newParentUpdate := t.Root()
	newParentPort := t.Parent()

	switch {
	case lastParentUpdate.RootPublicKey != newParentUpdate.RootPublicKey:
		fallthrough

	case lastParentUpdate.AncestorParent() != newParentUpdate.AncestorParent():
		fallthrough

	case lastParentPort != newParentPort:
		t.advertise()
		t.rootReset <- struct{}{}
	}
}

func (t *spanningTree) selectNewParent() {
	t.updateMutex.Lock()
	defer t.updateMutex.Unlock()

	lastParentUpdate := t.Root()

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

	if bestAnn != nil && bestAnn.RootPublicKey.CompareTo(t.r.public) > 0 {
		t.parent.Store(bestPort)
		newcoords, newroot := t.Coords(), t.Root().RootPublicKey

		if t.callback != nil && !lastParentUpdate.Coords().EqualTo(newcoords) {
			defer t.callback(bestPort, newcoords)
		}

		if newroot != lastParentUpdate.RootPublicKey {
			defer t.r.snake.rootNodeChanged(newroot)
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
				t.selectNewParentAndAdvertise()
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
	if newUpdate.RootPublicKey.CompareTo(t.Root().RootPublicKey) < 0 {
		// The update that this peer sent us, for some reason, has a weaker
		// root than the one that we know about. Send an announcement back
		// to them with our stronger root and do nothing further with the
		// update. If our own root timed out then t.Root() will return an
		// update with our own key, so it's equivalent to seeing if our key
		// is stronger than theirs.
		p.announce <- struct{}{}
		return nil
	}

	if p.IsParent() {
		// Check if anything strange has happened to our parent that might
		// indicate a problem with their parent, like a sudden lengthening
		// of the path or a root key change altogether. If these cases happen
		// then it might be that the parent we have is no longer the best one
		// so we should try and select a new one.
		if lastPortUpdate := p.lastAnnouncement(); lastPortUpdate != nil {
			switch {
			case newUpdate.AncestorParent() != lastPortUpdate.AncestorParent(): // our ancestor's parent changed
				fallthrough

			case newUpdate.RootPublicKey.CompareTo(lastPortUpdate.RootPublicKey) < 0: // the root key got weaker
				fallthrough

			case len(newUpdate.Signatures) > len(lastPortUpdate.Signatures): // the path got suddenly longer
				t.selectNewParentAndAdvertise()
				return nil
			}
		}
	}

	if !updateParent {
		// At this point we're looking to see if there are any reasons
		// that we should select a new parent. By this point, the peer's
		// update has already been saved, so positive cases here will mean
		// that we may end up switching to this node.
		lastParentUpdate := t.Root()
		keyDeltaSinceLastParentUpdate := newUpdate.RootPublicKey.CompareTo(lastParentUpdate.RootPublicKey)

		switch {
		case time.Since(lastParentUpdate.at) > announcementTimeout:
			// We haven't seen a suitable root update recently from our parent,
			// so try and select a new parent.
			updateParent = true

		case keyDeltaSinceLastParentUpdate < 0:
			// The root key is weaker than our existing root so there's no point
			// in attempting parent selection.

		case keyDeltaSinceLastParentUpdate > 0:
			// The root key is stronger than our existing root, therefore this
			// is likely a good candidate to be a new parent.
			updateParent = true

		case keyDeltaSinceLastParentUpdate == 0:
			// The update is from the same root node, let's see if it matches
			// any other useful conditions.
			switch {
			case newUpdate.Sequence < lastParentUpdate.Sequence:
			// This is a replay of an earlier update, therefore we should
			// ignore it, even if it came from our parent node

			case len(newUpdate.Signatures) < len(lastParentUpdate.Signatures):
				// The path to the root is shorter than our last update, so
				// we'll accept it
				updateParent = true
			}
		}
	}

	switch {
	case updateParent:
		t.selectNewParentAndAdvertise()

	case p.IsParent():
		t.advertise()
		t.rootReset <- struct{}{}
	}

	return nil
}
