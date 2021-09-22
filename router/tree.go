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

	defer r.tree.UpdateParentIfNeeded(peer, newUpdate)
	return peer.updateAnnouncement(&newUpdate)
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
	ordering  uint64
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
	lastUpdate := t.Root()
	bestKey := lastUpdate.RootPublicKey
	bestSeq := lastUpdate.Sequence
	bestOrder := uint64(math.MaxUint64)
	var bestPort types.SwitchPortID
	var bestAnn *rootAnnouncementWithTime

	t.mutex.Lock()

	active := t.r.activePorts()
	defer peersPool.Put(active) // nolint:staticcheck

	for _, p := range active {
		ann := p.lastAnnouncement()
		if ann == nil {
			continue
		}
		accept := func() {
			bestKey = ann.RootPublicKey
			bestPort = p.port
			bestOrder = ann.receiveOrder
			bestSeq = ann.Sequence
			bestAnn = ann
		}
		keyDelta := ann.RootPublicKey.CompareTo(bestKey)
		switch {
		case ann.IsLoopOrChildOf(p.r.public):
			// ignore our children or loopy announcements
		case keyDelta > 0:
			accept()
		case keyDelta < 0:
			// ignore weaker root keys
		case ann.Sequence > bestSeq:
			accept()
		case ann.Sequence < bestSeq:
			// ignore lower sequence numbers
		case ann.receiveOrder < bestOrder:
			accept()
		}
	}

	if bestAnn != nil {
		t.parent = bestPort
		t.mutex.Unlock()

		newCoords := bestAnn.Coords()
		coordsChanged := !lastUpdate.Coords().EqualTo(bestAnn.Coords())
		rootChanged := bestAnn.RootPublicKey != lastUpdate.RootPublicKey

		if rootChanged {
			t.r.snake.rootNodeChanged(bestAnn.RootPublicKey)
		}
		if rootChanged || coordsChanged {
			t.callback(bestPort, newCoords)
		}

		t.advertise()
		return
	}

	// No suitable other peer was found, so we'll just become the root
	// and hope that one of our peers corrects us if it matters.
	t.mutex.Unlock()
	t.becomeRoot()
}

func (t *spanningTree) advertise() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.r.ports[t.parent].mutex.RLock()
	ann := t.r.ports[t.parent].announcement
	t.r.ports[t.parent].mutex.RUnlock()

	if t.parent == 0 || ann == nil { // we are the root
		ann = &rootAnnouncementWithTime{
			receiveTime: time.Now(),
			SwitchAnnouncement: types.SwitchAnnouncement{
				RootPublicKey: t.r.public,
				Sequence:      types.Varu64(t.sequence.Inc()),
			},
		}
	}

	started := t.r.startedPorts()
	defer peersPool.Put(started) // nolint:staticcheck
	for _, p := range started {
		p.protoOut.push(ann.ForPeer(p))
	}
}

func (t *spanningTree) becomeRoot() {
	oldCoords := t.Coords()
	newCoords := types.SwitchPorts{}

	t.mutex.Lock()
	if t.parent != 0 {
		defer t.r.snake.rootNodeChanged(t.r.public)
	}
	t.parent = 0
	t.mutex.Unlock()

	if !oldCoords.EqualTo(newCoords) {
		go t.callback(0, newCoords)
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
	return root.RootPublicKey.EqualTo(t.r.public) || time.Since(root.receiveTime) >= announcementTimeout
}

func (t *spanningTree) Root() *rootAnnouncementWithTime {
	if root := t.r.ports[t.Parent()].lastAnnouncement(); root != nil {
		return root
	}
	return &rootAnnouncementWithTime{
		receiveTime: time.Now(),
		SwitchAnnouncement: types.SwitchAnnouncement{
			RootPublicKey: t.r.public,
			Sequence:      types.Varu64(t.sequence.Load()),
		},
	}
}

func (t *spanningTree) Parent() types.SwitchPortID {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.parent
}

func (t *spanningTree) UpdateParentIfNeeded(p *Peer, newUpdate types.SwitchAnnouncement) {
	const immediately = time.Duration(0)
	reparentIn, becomeRoot := time.Duration(-1), false
	lastParentUpdate, isParent := t.Root(), p.IsParent()
	keyDeltaSinceLastParentUpdate := newUpdate.RootPublicKey.CompareTo(lastParentUpdate.RootPublicKey)

	switch {
	case keyDeltaSinceLastParentUpdate > 0:
		// The peer has sent us a key that is stronger than our last update.
		reparentIn = immediately

	case time.Since(lastParentUpdate.receiveTime) >= announcementTimeout:
		// It's been a while since we last heard from our parent so we should
		// really choose a new one if we can.
		reparentIn = immediately

	case isParent && keyDeltaSinceLastParentUpdate < 0:
		// Our parent sent us a weaker key than before — this implies that
		// something bad happened.
		reparentIn, becomeRoot = time.Second/2, true

	case isParent && keyDeltaSinceLastParentUpdate == 0 && newUpdate.Sequence < lastParentUpdate.Sequence:
		// Our parent sent us a lower sequence number than before for the
		// same root — this isn't good news either.
		reparentIn, becomeRoot = time.Second/2, true

	case isParent && keyDeltaSinceLastParentUpdate == 0 && newUpdate.Sequence == lastParentUpdate.Sequence:
		// Our parent sent us an equal sequence number, which probably means
		// that their path to the root has changed. This isn't as bad news as
		// a weaker key but we should still try to find a new parent.
		reparentIn, becomeRoot = time.Second/4, false
	}

	switch {
	case reparentIn >= 0:
		if becomeRoot {
			t.becomeRoot()
		}
		t.selectNewParentAndAdvertiseIn(reparentIn)

	case isParent && keyDeltaSinceLastParentUpdate == 0 && newUpdate.Sequence > lastParentUpdate.Sequence:
		t.advertise()
		t.rootReset <- struct{}{}

	case keyDeltaSinceLastParentUpdate < 0:
		// This peer sent us an update that is weaker than our own root, and
		// none of the other conditions have been met, so the best thing to do
		// is to announce our root back to them quickly so that they will
		// hopefully converge.
		p.protoOut.push(lastParentUpdate.ForPeer(p))
	}
}
