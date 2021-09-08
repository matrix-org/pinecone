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

func (r *Router) handleAnnouncement(peer *Peer, rx *types.Frame) error {
	var newUpdate types.SwitchAnnouncement
	if _, err := newUpdate.UnmarshalBinary(rx.Payload); err != nil {
		return fmt.Errorf("failed to unmarshal root announcement: %w", err)
	}
	sigs := make(map[string]struct{})
	for index, sig := range newUpdate.Signatures {
		if index == 0 && sig.PublicKey != newUpdate.RootPublicKey {
			// The first signature in the announcement must be from the
			// key that claims to be the root.
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

	lastPortUpdate := peer.lastAnnouncement()
	if lastPortUpdate != nil && lastPortUpdate.RootPublicKey == newUpdate.RootPublicKey {
		if newUpdate.Sequence < lastPortUpdate.Sequence {
			// The update is a replay of a previous announcement which doesn't
			// make sense
			return fmt.Errorf("root announcement is a replay of an old sequence number")
		}
	}

	if err := peer.updateAnnouncement(&newUpdate); err != nil {
		return fmt.Errorf("updating the peer last root announcement failed: %w", err)
	}

	r.tree.UpdateParentIfNeeded(peer, newUpdate)
	return nil
}

type rootAnnouncementWithTime struct {
	types.SwitchAnnouncement
	at time.Time
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
	var payload [MaxPayloadSize]byte
	n, err := announcement.MarshalBinary(payload[:])
	if err != nil {
		p.r.log.Println("Failed to marshal switch announcement:", err)
		return nil
	}
	frame := types.GetFrame()
	frame.Version = types.Version0
	frame.Type = types.TypeSTP
	frame.Destination = types.SwitchPorts{}
	frame.Payload = payload[:n]
	return frame
}

type spanningTree struct {
	r         *Router
	context   context.Context
	rootReset chan struct{}
	mutex     *sync.Mutex
	parent    types.SwitchPortID
	reparent  *time.Timer
	sequence  atomic.Uint64
	callback  func(parent types.SwitchPortID, coords types.SwitchPorts)
}

func newSpanningTree(r *Router, f func(parent types.SwitchPortID, coords types.SwitchPorts)) *spanningTree {
	t := &spanningTree{
		r:         r,
		context:   r.context,
		rootReset: make(chan struct{}),
		mutex:     &sync.Mutex{},
		callback:  f,
	}
	t.reparent = time.AfterFunc(time.Second, t.selectNewParentAndAdvertise)
	t.reparent.Stop()
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
		t.selectNewParentAndAdvertiseIn(0)
	}
}

func (t *spanningTree) selectNewParentAndAdvertiseIn(d time.Duration) {
	t.reparent.Reset(d)
}

func (t *spanningTree) selectNewParentAndAdvertise() {
	lastParentUpdate := t.Root()
	lastParentPort := t.Parent()

	if !t.selectNewParent() {
		// We became the root so no point in continuing.
		return
	}

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

func (t *spanningTree) selectNewParent() bool {
	lastUpdate := t.Root()
	bestDist := math.MaxInt32
	bestKey := t.r.public
	bestTime := t.Root().at
	var bestPort types.SwitchPortID
	var bestAnn *rootAnnouncementWithTime
	var bestSeq types.Varu64

	t.mutex.Lock()

	for _, p := range t.r.activePorts() {
		if p.child.Load() {
			continue
		}
		ann := p.lastAnnouncement()
		if ann == nil {
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
		keyDelta := ann.RootPublicKey.CompareTo(bestKey)
		annAt := ann.at.Round(time.Millisecond)
		switch {
		case keyDelta > 0:
			accept()
		case keyDelta < 0:
			// ignore weaker root keys
		case ann.Sequence > bestSeq:
			accept()
		case ann.Sequence < bestSeq:
			// ignore lower sequence numbers
		case annAt.Before(bestTime):
			accept()
		case annAt.After(bestTime):
			// ignore updates that arrived more recently
		case len(ann.Signatures) < bestDist:
			accept()
		case len(ann.Signatures) > bestDist:
			// ignore longer paths
		case p.public.CompareTo(bestKey) > 0:
			accept()
		}
	}

	if bestAnn != nil {
		switch {
		case bestAnn.RootPublicKey.CompareTo(t.r.public) == 0 && bestSeq >= lastUpdate.Sequence:
			fallthrough

		case bestAnn.RootPublicKey.CompareTo(t.r.public) > 0:
			t.parent = bestPort
			t.mutex.Unlock()

			newAnn := t.Root()
			newCoords, newRoot := newAnn.Coords(), newAnn.RootPublicKey

			if t.callback != nil && !lastUpdate.Coords().EqualTo(newCoords) {
				defer t.callback(bestPort, newCoords)
			}

			if newRoot != lastUpdate.RootPublicKey {
				defer t.r.snake.rootNodeChanged(newRoot)
			}

			return true
		}
	}

	t.mutex.Unlock()
	t.becomeRoot()

	return false
}

func (t *spanningTree) advertise() {
	if t.IsRoot() {
		t.sequence.Inc()
	}
	ann := t.Root()
	for _, p := range t.r.startedPorts() {
		p.protoOut.push(ann.ForPeer(p))
	}
}

func (t *spanningTree) becomeRoot() {
	t.mutex.Lock()
	t.parent = 0
	for _, p := range t.r.ports {
		p.child.Store(false)
	}
	t.mutex.Unlock()

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
				t.becomeRoot()
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
				Sequence:      types.Varu64(t.sequence.Load()),
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
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.parent
}

func (t *spanningTree) UpdateParentIfNeeded(p *Peer, newUpdate types.SwitchAnnouncement) {
	updateParent, gotBadNews := false, false
	lastParentUpdate, isParent := t.Root(), p.IsParent()
	keyDeltaSinceLastParentUpdate := newUpdate.RootPublicKey.CompareTo(lastParentUpdate.RootPublicKey)

	switch {
	case keyDeltaSinceLastParentUpdate > 0:
		// The peer has sent us a key that is stronger than our last update.
		updateParent = true

	case time.Since(lastParentUpdate.at) >= announcementTimeout:
		// It's been a while since we last heard from our parent so we should
		// really choose a new one if we can.
		updateParent = true

	case isParent && keyDeltaSinceLastParentUpdate < 0:
		// Our parent sent us a weaker key than before — this implies that
		// something bad happened.
		gotBadNews = true

	case isParent && keyDeltaSinceLastParentUpdate == 0:
		if newUpdate.Sequence <= lastParentUpdate.Sequence {
			// Our parent sent us a lower sequence number than before for the
			// same root — this isn't good news either.
			gotBadNews = true
		}
	}

	switch {
	case gotBadNews:
		t.becomeRoot()
		t.selectNewParentAndAdvertiseIn(time.Second)
		return

	case updateParent:
		t.selectNewParentAndAdvertise()
		return

	case p.IsParent():
		switch {
		case newUpdate.RootPublicKey != lastParentUpdate.RootPublicKey:
			// The parent's root key has changed, for better or worse.
			fallthrough

		case newUpdate.RootPublicKey == lastParentUpdate.RootPublicKey && newUpdate.Sequence > lastParentUpdate.Sequence:
			// The parent's root key has not changed but we've got a nice
			// new sequence number.
			t.advertise()
			t.rootReset <- struct{}{}

		default:
			// If we aren't notifying anyone else, we should at least
			// notify our parent that we chose them as a parent.
			p.protoOut.push(t.Root().ForPeer(p))
		}
	}
}
