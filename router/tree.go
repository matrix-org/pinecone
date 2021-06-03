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
	parent := t.parent.Load().(types.SwitchPortID)
	if parent == port {
		var bestDist int
		var bestTime time.Time
		var bestPort types.SwitchPortID
		for _, p := range t.r.activePorts() {
			ann := p.lastAnnouncement()
			if ann == nil {
				continue
			}
			hops := len(ann.Signatures)
			switch {
			case ann.RootPublicKey != t.r.RootPublicKey():
				continue
			case bestDist == 0 || hops < bestDist:
				fallthrough
			case ann.at.Before(bestTime):
				bestDist = hops
				bestPort = p.port
				bestTime = ann.at
			}
		}
		t.parent.Store(bestPort)
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
	ticker := time.NewTicker(announcementInterval)
	advertise := func() {
		for _, p := range t.r.startedPorts() {
			p.announce.Dispatch()
		}
	}
	for {
		select {
		case <-t.context.Done():
			ticker.Stop()
			return

		case <-ticker.C:
			if t.IsRoot() {
				advertise()
			}

		case <-t.advertise:
			advertise()
			//ticker.Reset(announcementInterval)
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

func (t *spanningTree) Update(p *Peer, newUpdate types.SwitchAnnouncement) error {
	sigs := make(map[string]struct{})
	isLoopbackUpdate := false
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
		if sig.PublicKey.EqualTo(t.r.public) {
			// A child update is one that contains our public key in the
			// signatures already - it's probably one of our direct peers
			// sending our root announcement back to us. In this case there
			// is some special behaviour: we usually will need to accept
			// the update on the port, but we don't want to do anything that
			// would influence root or coordinate changes.
			isLoopbackUpdate = true
		}
		pk := hex.EncodeToString(sig.PublicKey[:])
		if _, ok := sigs[pk]; ok {
			// One of the signatures has appeared in the update more than
			// once, which would suggest that there's a loop somewhere.
			return fmt.Errorf("rejecting update (detected routing loop)")
		}
		sigs[pk] = struct{}{}
	}

	p.updateAnnouncement(&newUpdate)

	if isLoopbackUpdate {
		// The update contains our own signature already, so using it for
		// a root update would create a loop
		return nil
	}

	lastGlobalUpdate := t.Root()
	globalKeyDelta := newUpdate.RootPublicKey.CompareTo(lastGlobalUpdate.RootPublicKey)
	globalTimeSince := time.Since(lastGlobalUpdate.at)
	globalUpdate := false

	switch {
	case globalTimeSince > announcementTimeout:
		// We haven't seen a suitable root update recently so we'll accept
		// this one instead
		globalUpdate = true

	case globalKeyDelta < 0:
		// The root key is weaker than our existing root, so it's no good
		return nil

	case globalKeyDelta == 0:
		// The update is from the same root node, let's see if it matches
		// any other useful conditions

		switch {
		case newUpdate.Sequence < lastGlobalUpdate.Sequence:
			// This is a replay of an earlier update, therefore we should
			// ignore it, even if it came from our parent node
			return nil

		case len(newUpdate.Signatures) < len(lastGlobalUpdate.Signatures):
			// The path to the root is shorter than our last update, so
			// we'll accept it
			globalUpdate = true
		}

	case globalKeyDelta > 0:
		// The root key is stronger than our existing root, therefore we'll
		// accept it anyway, since we always want to converge on the
		// strongest root key
		globalUpdate = true
	}

	if parent := t.parent.Load(); p.port == parent || globalUpdate {
		t.rootMutex.Lock()
		var oldRootKey types.PublicKey
		if t.root != nil {
			oldRootKey = t.root.RootPublicKey
		}
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

		if oldRootKey != newUpdate.RootPublicKey {
			go t.r.snake.rootNodeChanged(newUpdate.RootPublicKey)
		}

		t.advertise.Dispatch()
	}

	return nil
}
