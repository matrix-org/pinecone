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
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
)

// NOTE: Functions prefixed with an underscore (_) are only safe to be called
// from the actor that owns them, in order to prevent data races.

const virtualSnakeMaintainInterval = time.Second * 2
const virtualSnakeNeighExpiryPeriod = time.Second * 5

type virtualSnakeTable map[virtualSnakeIndex]*virtualSnakeEntry

type virtualSnakeIndex struct {
	PublicKey types.PublicKey `json:"public_key"`
}

type virtualSnakeEntry struct {
	*virtualSnakeIndex
	Origin   types.PublicKey `json:"origin"`
	Target   types.PublicKey `json:"target"`
	Source   *peer           `json:"source"`
	LastSeen time.Time       `json:"last_seen"`
	Root     types.Root      `json:"root"`
	Active   bool            `json:"active"`
}

// valid returns true if the update hasn't expired, or false if it has. It is
// required for updates to time out eventually, in the case that paths don't get
// torn down properly for some reason.
func (e *virtualSnakeEntry) valid() bool {
	return time.Since(e.LastSeen) < virtualSnakeNeighExpiryPeriod
}

func (e *virtualSnakeEntry) validForForwarding() bool {
	return time.Since(e.LastSeen) < (virtualSnakeNeighExpiryPeriod * 2)
}

// _maintainSnake is responsible for working out if we need to send bootstraps
// or to clean up any old paths.
func (s *state) _maintainSnake() {
	select {
	case <-s.r.context.Done():
		return
	default:
		defer s._maintainSnakeIn(virtualSnakeMaintainInterval)
	}

	// Work out if we are able to bootstrap. If we are the root node then
	// we don't send bootstraps, since there's nowhere for them to go —
	// bootstraps are sent up to the next ascending node, but as the root,
	// we already have the highest key on the network.
	rootAnn := s._rootAnnouncement()

	// The descending node is the node with the next lowest key.
	if desc := s._descending; desc != nil {
		switch {
		case !desc.valid():
			fallthrough
		case !desc.Root.EqualTo(&rootAnn.Root):
			s._setDescendingNode(nil)
		}
	}

	// Clean up any paths that were installed more than 5 seconds ago but haven't
	// been activated by a setup ACK.
	for k, v := range s._table {
		if !v.valid() {
			delete(s._table, k)
		}
	}

	// If one of the previous conditions means that we need to bootstrap, then
	// send the actual bootstrap message into the network.
	s._bootstrapNow()
}

// _bootstrapNow is responsible for sending a bootstrap message to the network.
func (s *state) _bootstrapNow() {
	// If we are the root node then there's no point in trying to bootstrap. We
	// already have the highest public key on the network so a bootstrap won't be
	// able to go anywhere in ascending order.
	if s._parent == nil {
		return
	}
	// Construct the bootstrap packet. We will include our root key and sequence
	// number in the update so that the remote side can determine if we are both using
	// the same root node when processing the update.
	ann := s._rootAnnouncement()
	b := frameBufferPool.Get().(*[types.MaxFrameSize]byte)
	defer frameBufferPool.Put(b)
	bootstrap := types.VirtualSnakeBootstrap{
		Root: ann.Root,
	}
	// Generate a random path ID.
	if _, err := rand.Read(bootstrap.PathID[:]); err != nil {
		return
	}
	if s.r.secure {
		// Sign the path key and path ID with our own key. This forms the "source
		// signature", which anyone can use to verify that we sent the bootstrap.
		copy(
			bootstrap.SourceSig[:],
			ed25519.Sign(s.r.private[:], append(s.r.public[:], bootstrap.PathID[:]...)),
		)
	}
	n, err := bootstrap.MarshalBinary(b[:])
	if err != nil {
		return
	}
	// Construct the frame. We set the destination key to be our own public key. As
	// the bootstrap routing defaults to routing towards higher keys, this should
	// mean that the message gets forwarded up to the next highest key from ours.
	send := getFrame()
	send.Type = types.TypeVirtualSnakeBootstrap
	send.DestinationKey = s.r.public
	send.Source = s._coords()
	send.Payload = append(send.Payload[:0], b[:n]...)
	// Bootstrap messages are routed using SNEK routing with special rules for
	// bootstrap packets.
	if p := s._nextHopsSNEK(send, true); p != nil && p.proto != nil {
		p.proto.push(send)
	}
}

type virtualSnakeNextHopParams struct {
	isBootstrap       bool
	isTraffic         bool
	destinationKey    types.PublicKey
	publicKey         types.PublicKey
	parentPeer        *peer
	selfPeer          *peer
	lastAnnouncement  *rootAnnouncementWithTime
	peerAnnouncements announcementTable
	snakeRoutes       virtualSnakeTable
}

// _nextHopsSNEK locates the best next-hop for a given SNEK-routed frame. The
// bootstrap flag determines whether the frame should be routed using bootstrap
// specific rules — this should only be used for VirtualSnakeBootstrap frames.
func (s *state) _nextHopsSNEK(rx *types.Frame, bootstrap bool) *peer {
	return getNextHopSNEK(virtualSnakeNextHopParams{
		bootstrap,
		rx.Type == types.TypeVirtualSnakeRouted || rx.Type == types.TypeTreeRouted,
		rx.DestinationKey,
		s.r.public,
		s._parent,
		s.r.local,
		s._rootAnnouncement(),
		s._announcements,
		s._table,
	})
}

func getNextHopSNEK(params virtualSnakeNextHopParams) *peer {
	// If the message isn't a bootstrap message and the destination is for our
	// own public key, handle the frame locally — it's basically loopback.
	if !params.isBootstrap && params.publicKey == params.destinationKey {
		return params.selfPeer
	}

	// We start off with our own key as the best key. Any suitable next-hop
	// candidate has to improve on our own key in order to forward the frame.
	var bestPeer *peer
	var bestAnn *rootAnnouncementWithTime
	if params.isTraffic {
		bestPeer = params.selfPeer
	}
	bestKey := params.publicKey
	destKey := params.destinationKey

	// newCandidate updates the best key and best peer with new candidates.
	newCandidate := func(key types.PublicKey, p *peer) {
		bestKey, bestPeer, bestAnn = key, p, params.peerAnnouncements[p]
	}
	// newCheckedCandidate performs some sanity checks on the candidate before
	// passing it to newCandidate.
	newCheckedCandidate := func(candidate types.PublicKey, p *peer) {
		switch {
		case !params.isBootstrap && candidate == destKey && bestKey != destKey:
			newCandidate(candidate, p)
		case util.DHTOrdered(destKey, candidate, bestKey):
			newCandidate(candidate, p)
		}
	}

	// Check if we can use the path to the root via our parent as a starting
	// point. We can't do this if we are the root node as there would be no
	// parent or ascending paths.
	if params.parentPeer != nil && params.parentPeer.started.Load() {
		switch {
		case params.isBootstrap && bestKey == destKey:
			// Bootstraps always start working towards thear root so that they
			// go somewhere rather than getting stuck.
			fallthrough
		case util.DHTOrdered(bestKey, destKey, params.lastAnnouncement.RootPublicKey):
			// The destination key is higher than our own key, so start using
			// the path to the root as the first candidate.
			newCandidate(params.lastAnnouncement.RootPublicKey, params.parentPeer)
		}

		// Check our direct ancestors in the tree, that is, all nodes between
		// ourselves and the root node via the parent port.
		if ann := params.peerAnnouncements[params.parentPeer]; ann != nil {
			for _, ancestor := range ann.Signatures {
				newCheckedCandidate(ancestor.PublicKey, params.parentPeer)
			}
		}
	}

	// Check all of the ancestors of our direct peers too, that is, all nodes
	// between our direct peer and the root node.
	for p, ann := range params.peerAnnouncements {
		if !p.started.Load() {
			continue
		}
		for _, hop := range ann.Signatures {
			newCheckedCandidate(hop.PublicKey, p)
		}
	}

	// Check whether our current best candidate is actually a direct peer.
	// This might happen if we spotted the node in our direct ancestors for
	// example, only in this case it would make more sense to route directly
	// to the peer via our peering with them as opposed to routing via our
	// parent port.
	for p := range params.peerAnnouncements {
		if !p.started.Load() {
			continue
		}
		if peerKey := p.public; bestKey == peerKey {
			// We've seen this key already and we are directly peered, so use
			// the peering instead of the previous selected port.
			newCandidate(peerKey, p)
		}
	}

	// Check our DHT entries. In particular, we are only looking at the source
	// side of the DHT paths. Since setups travel from the lower key to the
	// higher one, this is effectively looking for paths that descend through
	// keyspace toward lower keys rather than ascend toward higher ones.
	for _, entry := range params.snakeRoutes {
		if !entry.Source.started.Load() || !entry.validForForwarding() || entry.Source == params.selfPeer {
			continue
		}
		if !params.isBootstrap && !entry.Active {
			continue
		}
		newCheckedCandidate(entry.PublicKey, entry.Source)
	}

	// Finally, be sure that we're using the best-looking path to our next-hop.
	// Prefer faster link types and, if not, lower latencies to the root.
	if bestPeer != nil && bestAnn != nil {
		for p, ann := range params.peerAnnouncements {
			peerKey := p.public
			switch {
			case bestKey != peerKey:
				continue
			case p.peertype < bestPeer.peertype:
				// Prefer faster classes of links if possible.
				newCandidate(peerKey, p)
			case p.peertype == bestPeer.peertype && ann.receiveOrder < bestAnn.receiveOrder:
				// Prefer links that have the lowest latency to the root.
				newCandidate(peerKey, p)
			}
		}
	}

	return bestPeer
}

// _handleBootstrap is called in response to receiving a bootstrap packet.
// This function will send a bootstrap ACK back to the sender.
func (s *state) _handleBootstrap(from *peer, rx *types.Frame) error {
	// Unmarshal the bootstrap.
	var bootstrap types.VirtualSnakeBootstrap
	if _, err := bootstrap.UnmarshalBinary(rx.Payload); err != nil {
		return fmt.Errorf("bootstrap.UnmarshalBinary: %w", err)
	}
	if s.r.secure {
		// Check that the bootstrap message was signed by the node that claims
		// to have sent it. Silently drop it if there's a signature problem.
		if !ed25519.Verify(
			rx.DestinationKey[:],
			append(rx.DestinationKey[:], bootstrap.PathID[:]...),
			bootstrap.SourceSig[:],
		) {
			return nil
		}
	}
	// Check that the root key and sequence number in the update match our
	// current root, otherwise we won't be able to route back to them using
	// tree routing anyway. If they don't match, silently drop the bootstrap.
	root := s._rootAnnouncement()
	if !root.Root.EqualTo(&bootstrap.Root) {
		return nil
	}

	// Create a routing table entry.
	index := virtualSnakeIndex{
		PublicKey: rx.DestinationKey,
	}
	s._table[index] = &virtualSnakeEntry{
		virtualSnakeIndex: &index,
		Origin:            rx.DestinationKey,
		Target:            rx.DestinationKey,
		Source:            from,
		LastSeen:          time.Now(),
		Root:              bootstrap.Root,
		Active:            true,
	}

	// Now let's see if this is a suitable ascending entry.
	update := false
	desc := s._descending
	switch {
	case !root.Root.EqualTo(&bootstrap.Root):
		// The root key in the bootstrap ACK doesn't match our own key, or the
		// sequence doesn't match, so it is quite possible that routing setup packets
		// using tree routing would fail.
	case !util.LessThan(rx.DestinationKey, s.r.public):
		// The bootstrapping key should be less than ours but it isn't.
	case desc != nil && desc.valid():
		// We already have a descending entry and it hasn't expired.
		switch {
		case desc.PublicKey == rx.DestinationKey:
			// We've received another bootstrap from our direct descending node.
			// Send back an acknowledgement as this is OK.
			update = true
		case util.DHTOrdered(desc.PublicKey, rx.DestinationKey, s.r.public):
			// The bootstrapping node is closer to us than our previous descending
			// node was.
			update = true
		}
	case desc == nil || !desc.valid():
		// We don't have a descending entry, or we did but it expired.
		if util.LessThan(rx.DestinationKey, s.r.public) {
			// The bootstrapping key is less than ours so we'll acknowledge it.
			update = true
		}
	default:
		// The bootstrap conditions weren't met. This might just be because
		// there's a node out there that hasn't converged to a closer node
		// yet, so we'll just ignore the bootstrap.
	}
	if update {
		s._setDescendingNode(s._table[index])
	}
	return nil
}
