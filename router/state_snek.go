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
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
)

// NOTE: Functions prefixed with an underscore (_) are only safe to be called
// from the actor that owns them, in order to prevent data races.

const virtualSnakeMaintainInterval = time.Second / 2
const virtualSnakeBootstrapMinInterval = time.Second
const virtualSnakeBootstrapInterval = virtualSnakeBootstrapMinInterval * 2
const virtualSnakeNeighExpiryPeriod = virtualSnakeBootstrapInterval * 2

type virtualSnakeTable map[virtualSnakeIndex]*virtualSnakeEntry

type virtualSnakeIndex struct {
	PublicKey types.PublicKey `json:"public_key"`
}

type virtualSnakeEntry struct {
	*virtualSnakeIndex
	Source      *peer                       `json:"source"`
	Destination *peer                       `json:"destination"`
	Watermark   types.VirtualSnakeWatermark `json:"watermark"`
	LastSeen    time.Time                   `json:"last_seen"`
}

// valid returns true if the update hasn't expired, or false if it has. It is
// required for updates to time out eventually, in the case that paths don't get
// torn down properly for some reason.
func (e *virtualSnakeEntry) valid() bool {
	return time.Since(e.LastSeen) < virtualSnakeNeighExpiryPeriod
}

// _maintainSnake is responsible for working out if we need to send bootstraps
// or to clean up any old paths.
func (s *state) _maintainSnake() {
	select {
	case <-s.r.context.Done():
		return
	default:
		defer s._maintainSnakeIn(virtualSnakeMaintainInterval)
		if s._peercount == 0 {
			return
		}
	}

	// The descending node is the node with the next lowest key.
	if desc := s._descending; desc != nil && !desc.valid() {
		s._setDescendingNode(nil)
	}

	// Clean up any paths that are older than the expiry period.
	for k, v := range s._table {
		if !v.valid() {
			delete(s._table, k)
		}
	}

	// Send a new bootstrap.
	if time.Since(s._lastbootstrap) >= s._interval {
		s._bootstrapNow()
	}
	if s._interval < virtualSnakeBootstrapInterval {
		s._interval += time.Second / 2
	}
}

// _bootstrapSoon will reset the bootstrap timer so that we will bootstrap on
// the next maintenance interval. This is better than calling _bootstrapNow
// directly which might cause more protocol traffic than necessary.
func (s *state) _bootstrapSoon() {
	s._lastbootstrap = time.Now().Add(-virtualSnakeBootstrapInterval)
}

// _bootstrapNow is responsible for sending a bootstrap message to the network.
func (s *state) _bootstrapNow() {
	// Construct the bootstrap packet.
	b := frameBufferPool.Get().(*[types.MaxFrameSize]byte)
	defer frameBufferPool.Put(b)
	bootstrap := types.VirtualSnakeBootstrap{
		Sequence: types.Varu64(time.Now().UnixMilli()),
	}
	if s.r.secure {
		protected, err := bootstrap.ProtectedPayload()
		if err != nil {
			return
		}
		copy(
			bootstrap.Signature[:],
			ed25519.Sign(s.r.private[:], protected),
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
	send.Payload = append(send.Payload[:0], b[:n]...)
	send.Watermark = types.VirtualSnakeWatermark{
		PublicKey: types.FullMask,
		Sequence:  0,
	}

	// If we think we're the highest key in the network then we'll send our bootstrap
	// out to all peers and tell them all of our existence. If they also believe us
	// to be the highest key on the network then they will repeat it to their peers.
	// Otherwise, we expect to be told from our peers that we're not the highest, in
	// which case we'll just bootstrap normally.
	if highest := s._getHighest(); highest.PublicKey == s.r.public {
		s._flood(s.r.local, send)
		framePool.Put(send)
	} else if p, w := s._nextHopsSNEK(send.DestinationKey, types.TypeVirtualSnakeBootstrap, send.Watermark); p != nil && p.proto != nil {
		send.Watermark = w
		p.proto.push(send)
	}
	s._lastbootstrap = time.Now()
}

type virtualSnakeNextHopParams struct {
	isBootstrap    bool
	peers          []*peer
	highest        *virtualSnakeEntry
	destinationKey types.PublicKey
	publicKey      types.PublicKey
	watermark      types.VirtualSnakeWatermark
	selfPeer       *peer
	snakeRoutes    virtualSnakeTable
}

// _nextHopsSNEK locates the best next-hop for a given SNEK-routed frame.
func (s *state) _nextHopsSNEK(dest types.PublicKey, frameType types.FrameType, watermark types.VirtualSnakeWatermark) (*peer, types.VirtualSnakeWatermark) {
	return getNextHopSNEK(virtualSnakeNextHopParams{
		frameType == types.TypeVirtualSnakeBootstrap,
		s._peers,
		s._getHighest(),
		dest,
		s.r.public,
		watermark,
		s.r.local,
		s._table,
	})
}

func getNextHopSNEK(params virtualSnakeNextHopParams) (*peer, types.VirtualSnakeWatermark) {
	// If the message isn't a bootstrap message and the destination is for our
	// own public key, handle the frame locally — it's basically loopback.
	if !params.isBootstrap && params.publicKey == params.destinationKey {
		return params.selfPeer, params.watermark
	}

	// We start off with our own key as the best key. Any suitable next-hop
	// candidate has to improve on our own key in order to forward the frame.
	var bestPeer *peer
	var bestSeq types.Varu64
	if !params.isBootstrap {
		bestPeer = params.selfPeer
	}
	bestKey := params.publicKey
	destKey := params.destinationKey
	watermark := params.watermark

	// newCandidate updates the best key and best peer with new candidates.
	newCandidate := func(key types.PublicKey, seq types.Varu64, p *peer) {
		bestKey, bestSeq, bestPeer = key, seq, p
	}
	// newCheckedCandidate performs some sanity checks on the candidate before
	// passing it to newCandidate.
	newCheckedCandidate := func(candidate types.PublicKey, seq types.Varu64, p *peer) {
		switch {
		case !params.isBootstrap && candidate == destKey && bestKey != destKey:
			newCandidate(candidate, seq, p)
		case util.DHTOrdered(destKey, candidate, bestKey):
			newCandidate(candidate, seq, p)
		}
	}

	// Start off with a path to the highest key that we know of.
	if params.highest != nil {
		switch {
		case params.isBootstrap && bestKey == destKey:
			// Bootstraps always start working towards our highest key so they
			// go somewhere instead of getting stuck.
			fallthrough
		case util.DHTOrdered(bestKey, destKey, params.highest.PublicKey):
			// The destination key is higher than our own key, so start using
			// the path to the highest key as the first candidate.
			newCandidate(params.highest.PublicKey, 0, params.highest.Source)
		}
	}

	// Check all of our direct peers, in case any of them provide a better path.
	for _, p := range params.peers {
		if p == nil || !p.started.Load() {
			continue
		}
		newCheckedCandidate(p.public, 0, p)
	}

	// Check our DHT entries. In particular, we are only looking at the source
	// side of the DHT paths. Since setups travel from the lower key to the
	// higher one, this is effectively looking for paths that descend through
	// keyspace toward lower keys rather than ascend toward higher ones.
	for _, entry := range params.snakeRoutes {
		if !entry.Source.started.Load() || !entry.valid() {
			continue
		}
		if entry.Watermark.WorseThan(watermark) {
			continue
		}
		newCheckedCandidate(entry.PublicKey, entry.Watermark.Sequence, entry.Source)
	}

	// Only SNEK paths will have a sequence number higher than 0, so
	// it's a safe bet that if it's greater than 0, we have hit upon
	// a newly watermarkable path.
	if bestSeq > 0 {
		watermark = types.VirtualSnakeWatermark{
			PublicKey: bestKey,
			Sequence:  bestSeq,
		}
	}

	return bestPeer, watermark
}

// _handleBootstrap is called in response to receiving a bootstrap packet.
// Returns true if the bootstrap was handled and false otherwise.
func (s *state) _handleBootstrap(from, to *peer, rx *types.Frame) bool {
	// Unmarshal the bootstrap.
	var bootstrap types.VirtualSnakeBootstrap
	_, err := bootstrap.UnmarshalBinary(rx.Payload)
	if err != nil {
		return false
	}
	if s.r.secure {
		// Check that the bootstrap message was protected by the node that claims
		// to have sent it. Silently drop it if there's a signature problem.
		protected, err := bootstrap.ProtectedPayload()
		if err != nil {
			return false
		}
		if !ed25519.Verify(
			rx.DestinationKey[:],
			protected,
			bootstrap.Signature[:],
		) {
			return false
		}
	}

	// Create a routing table entry.
	index := virtualSnakeIndex{
		PublicKey: rx.DestinationKey,
	}
	if existing, ok := s._table[index]; ok && bootstrap.Sequence <= existing.Watermark.Sequence {
		// TODO: less than-equal to might not be the right thing to do
		return false
	}
	s._table[index] = &virtualSnakeEntry{
		virtualSnakeIndex: &index,
		Source:            from,
		Destination:       to,
		LastSeen:          time.Now(),
		Watermark: types.VirtualSnakeWatermark{
			PublicKey: index.PublicKey,
			Sequence:  bootstrap.Sequence,
		},
	}

	// Now let's see if this is a suitable descending entry.
	update := false
	desc := s._descending
	switch {
	case !util.LessThan(rx.DestinationKey, s.r.public):
		// The bootstrapping key should be less than ours but it isn't.
	case desc != nil && desc.valid():
		// We already have a descending entry and it hasn't expired.
		switch {
		case desc.PublicKey == rx.DestinationKey:
			// We've received another bootstrap from our direct descending node.
			// Accept the update if the sequence number is higher only.
			update = bootstrap.Sequence > desc.Watermark.Sequence
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

	// If this is a higher key than that which we've seen, update our
	// highest entry with it.
	highest := s._getHighest()
	diff := index.PublicKey.CompareTo(highest.PublicKey)
	switch {
	case diff < 0:
		// The bootstrap is for a path with a key lower then our
		// currently known highest key.
		break
	case diff == 0 && bootstrap.Sequence <= highest.Watermark.Sequence:
		// The bootstrap is for the same path but the bootstrap
		// number is out of date.
		break
	default:
		// The bootstrap is for a stronger key, or for the same key
		// but a newer sequence number.
		s._highest = s._table[index]
		s._flood(from, rx)
	}

	return true
}
