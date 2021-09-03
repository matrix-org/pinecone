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

	if err := r.tree.Update(peer, newUpdate); err != nil {
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
	reparenting atomic.Bool     //
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
	if t.Parent() == port && t.reparenting.CAS(false, true) {
		t.selectNewParentAndAdvertise()
	}
}

func (t *spanningTree) selectNewParentAndAdvertise() {
	t.reparenting.Store(false)

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
	}
}

func (t *spanningTree) selectNewParent() {
	t.updateMutex.Lock()
	defer t.updateMutex.Unlock()

	lastUpdate := t.Root()
	bestDist := math.MaxInt32
	bestKey := t.r.public
	bestTime := t.Root().at
	var bestPort types.SwitchPortID
	var bestAnn *rootAnnouncementWithTime
	var bestSeq types.Varu64

	for _, p := range t.r.activePorts() {
		if p.child.Load() {
			continue
		}
		ann := p.lastAnnouncement()
		if ann == nil {
			continue
		}
		if time.Since(ann.at) > announcementTimeout {
			continue
		}
		accept := func() {
			bestKey = ann.RootPublicKey
			bestDist = len(ann.Signatures)
			bestPort = p.port
			bestTime = ann.at
			bestSeq = ann.Sequence
			bestAnn = ann
		}
		annAt := ann.at.Round(time.Millisecond)
		keyDelta := ann.RootPublicKey.CompareTo(bestKey)
		switch {
		case keyDelta > 0:
			accept()
		case keyDelta < 0:
			// ignore
		case ann.Sequence > bestSeq:
			accept()
		case ann.Sequence < bestSeq:
			// ignore
		case len(ann.Signatures) < bestDist:
			accept()
		case len(ann.Signatures) > bestDist:
			// ignore
		case annAt.Before(bestTime):
			accept()
		case annAt.After(bestTime):
			// ignore
		case p.public.CompareTo(bestKey) > 0:
			accept()
		}
	}

	if bestAnn != nil && bestAnn.RootPublicKey.CompareTo(t.r.public) > 0 {
		t.parent.Store(bestPort)
		newcoords, newroot := t.Coords(), t.Root().RootPublicKey

		if t.callback != nil && !lastUpdate.Coords().EqualTo(newcoords) {
			defer t.callback(bestPort, newcoords)
		}

		if newroot != lastUpdate.RootPublicKey {
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
			if !t.IsRoot() && t.reparenting.CAS(false, true) {
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

func (t *spanningTree) strongestRootKeyOfPeers() types.PublicKey {
	strongest := t.r.public
	for _, p := range t.r.activePorts() {
		if p.announcement.RootPublicKey.CompareTo(strongest) > 0 {
			strongest = p.announcement.RootPublicKey
		}
	}
	return strongest
}

func (t *spanningTree) Update(p *Peer, newUpdate types.SwitchAnnouncement) error {
	updateParent := false
	lastParentUpdate := t.Root()
	keyDeltaSinceLastParentUpdate := newUpdate.RootPublicKey.CompareTo(lastParentUpdate.RootPublicKey)
	keyDeltaFromStrongestRootKeyOfPeers := newUpdate.RootPublicKey.CompareTo(t.strongestRootKeyOfPeers())

	switch {
	case time.Since(lastParentUpdate.at) > announcementTimeout:
		updateParent = true

	case keyDeltaFromStrongestRootKeyOfPeers < 0 && p.port == t.Parent():
		updateParent = true

	case keyDeltaSinceLastParentUpdate != 0:
		updateParent = true

	case keyDeltaSinceLastParentUpdate == 0:
		switch {
		case newUpdate.Sequence < lastParentUpdate.Sequence:
			// Ignore an update with an old sequence number

		case len(newUpdate.Signatures) != len(lastParentUpdate.Signatures):
			updateParent = true

		default:
			for i := 0; i < len(newUpdate.Signatures); i++ {
				if newUpdate.Signatures[i] != lastParentUpdate.Signatures[i] {
					updateParent = true
					break
				}
			}
		}
	}

	switch {
	case updateParent:
		if t.reparenting.CAS(false, true) {
			time.AfterFunc(time.Second, t.selectNewParentAndAdvertise)
		}

	case p.IsParent():
		if newUpdate.Sequence >= lastParentUpdate.Sequence {
			t.advertise()
		}
		if newUpdate.Sequence > lastParentUpdate.Sequence {
			t.rootReset <- struct{}{}
		}
	}

	return nil
}
