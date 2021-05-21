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
const announcementInterval = time.Second * 16

// announcementTimeout is the amount of time that must
// pass without receiving a root announcement before we
// will assume that the peer is dead.
const announcementTimeout = announcementInterval * 4

func (r *Router) handleAnnouncement(peer *Peer, rx *types.Frame) {
	var new types.SwitchAnnouncement
	if _, err := new.UnmarshalBinary(rx.Payload); err != nil {
		r.log.Println("Error unmarshalling announcement:", err)
		return
	}
	if err := r.tree.Update(peer, new); err != nil {
		r.log.Println("Error handling announcement on port", peer.port, ":", err)
	}
}

type rootAnnouncementWithTime struct {
	types.SwitchAnnouncement
	at time.Time
}

type spanningTree struct {
	r         *Router                   //
	context   context.Context           //
	advertise util.Dispatch             // advertise to all other ports except for this one
	root      *rootAnnouncementWithTime //
	rootMutex sync.RWMutex              //
	rootReset util.Dispatch             //
	parent    atomic.Value              // types.SwitchPortID
	coords    atomic.Value              // types.SwitchPorts
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
	go t.workerForRoot()
	go t.workerForAnnouncements()
	return t
}

func (t *spanningTree) Coords() types.SwitchPorts {
	coords, ok := t.coords.Load().(types.SwitchPorts)
	if ok {
		return coords
	}
	return types.SwitchPorts{}
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
}

func (t *spanningTree) becomeRoot() {
	t.parent.Store(types.SwitchPortID(0))
	newCoords := types.SwitchPorts{}
	if !t.Coords().EqualTo(newCoords) {
		t.callback(0, types.SwitchPorts{})
	}
	t.rootReset.Dispatch()
	t.advertise.Dispatch()
}

func (t *spanningTree) workerForAnnouncements() {
	advertise := func() {
		for _, p := range t.r.startedPorts() {
			go func(p *Peer) {
				p.protoOut <- p.generateAnnouncement()
			}(p)
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
	root := t.Root()
	return root.RootPublicKey.EqualTo(t.r.public) || time.Since(root.at) >= announcementTimeout
}

func (t *spanningTree) Root() *rootAnnouncementWithTime {
	t.rootMutex.RLock()
	root := t.root
	t.rootMutex.RUnlock()
	if root == nil || time.Since(root.at) > announcementTimeout {
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

func (t *spanningTree) Remove(p *Peer) {
	if t.Parent() == p.port {
		t.parent.Store(types.SwitchPortID(0))
	}
}

func (t *spanningTree) Update(p *Peer, newUpdate types.SwitchAnnouncement) error {
	lastGlobalUpdate := t.Root()
	lastPortUpdate := p.lastAnnouncement()
	if lastPortUpdate == nil {
		lastPortUpdate = &rootAnnouncementWithTime{}
	}

	portTimeSince := time.Since(lastPortUpdate.at)
	globalTimeSince := time.Since(lastGlobalUpdate.at)

	// ------ SANITY CHECK THE UPDATE ITSELF ------

	sigs := make(map[string]struct{})
	isChild := false
	for _, sig := range newUpdate.Signatures {
		if sig.Hop == 0 {
			// None of the hops in the update should have a port number of 0
			// as this would imply that another node has sent their router
			// port, which is impossible. We'll therefore reject any update
			// that tries to do that.
			return fmt.Errorf("rejecting update (invalid 0 hop)")
		}
		if sig.PublicKey.EqualTo(t.r.public) {
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

	// ------ SANITY CHECK THE PREVIOUS UPDATE FROM THIS PEER ------

	portKeyDelta := newUpdate.RootPublicKey.CompareTo(lastPortUpdate.RootPublicKey)

	switch {
	case portTimeSince > announcementTimeout:
	case globalTimeSince > announcementTimeout:
		// We haven't heard a root update in a while, so if that's the
		// case then we'll skip the port-local checks and skip straight
		// to the global ones

	case portKeyDelta < 0: // Weaker root key
		return fmt.Errorf("rejecting update (key is weaker than last update)")

	case portKeyDelta == 0: // Same root key
		switch {
		case portTimeSince < announcementThreshold:
			return fmt.Errorf("rejecting update (too soon from same key after %s)", portTimeSince)
		case newUpdate.Sequence <= lastPortUpdate.Sequence:
			return fmt.Errorf("rejecting update (replayed sequence %d <= %d)", newUpdate.Sequence, lastPortUpdate.Sequence)
		}

	case portKeyDelta > 0: // Stronger root key
	}

	p.updateAnnouncement(&newUpdate)

	// ------ SANITY CHECK THE ACTUAL ROOT UPDATE FOR THE ENTIRE NODE ITSELF ------

	globalKeyDelta := newUpdate.RootPublicKey.CompareTo(lastGlobalUpdate.RootPublicKey)
	globalUpdate := false

	switch {
	case isChild:
		// The update contains our own key, so it's been looped back to us,
		// so we can't use it as a path to the root

	case globalTimeSince > announcementTimeout:
		// The global announcement hasn't been updated recently so we'll
		// accept this update in the meantime
		globalUpdate = true

	case globalKeyDelta < 0:
		// The key is weaker than our existing root

	case globalKeyDelta == 0: // Same root key
		switch {
		case len(newUpdate.Signatures) < len(lastPortUpdate.Signatures):
			globalUpdate = true
		}

	case globalKeyDelta > 0: // Stronger root key
		globalUpdate = true
	}

	if parent := t.parent.Load(); p.port == parent || globalUpdate {
		t.rootMutex.Lock()

		t.root = &rootAnnouncementWithTime{
			at:                 time.Now(),
			SwitchAnnouncement: newUpdate,
		}
		coords := types.SwitchPorts{}
		for _, hop := range t.root.Signatures {
			coords = append(coords, types.SwitchPortID(hop.Hop))
		}
		t.parent.Store(p.port)
		t.coords.Store(coords)

		t.rootMutex.Unlock()
		t.rootReset.Dispatch()
	}

	if t.parent.Load() == p.port {
		t.advertise.Dispatch()
	}

	/*
		if newRoot != nil && newRoot.RootPublicKey != old.RootPublicKey {
			go t.r.snake.rootNodeChanged(newRoot.RootPublicKey)
		}
	*/

	return nil
}
