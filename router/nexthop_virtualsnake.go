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
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
)

const virtualSnakeNeighExpiryPeriod = time.Hour

type virtualSnake struct {
	r           *Router
	mutex       sync.Mutex
	_table      virtualSnakeTable
	_ascending  *virtualSnakeNeighbour
	_descending *virtualSnakeNeighbour
	bootstrap   *time.Timer
}

type virtualSnakeTable map[virtualSnakeIndex]virtualSnakeEntry

type virtualSnakeIndex struct {
	PublicKey types.PublicKey
	PathID    types.VirtualSnakePathID
}

type virtualSnakeEntry struct {
	SourcePort      types.SwitchPortID
	DestinationPort types.SwitchPortID
	LastSeen        time.Time
	RootPublicKey   types.PublicKey
}

func (e *virtualSnakeEntry) Valid() bool {
	// TODO: this should not be needed, since we should only remove
	// paths by teardown ideally
	return time.Since(e.LastSeen) < virtualSnakeNeighExpiryPeriod
}

type virtualSnakeNeighbour struct {
	PublicKey     types.PublicKey
	Port          types.SwitchPortID
	LastSeen      time.Time
	Coords        types.SwitchPorts
	PathID        types.VirtualSnakePathID
	RootPublicKey types.PublicKey
}

func newVirtualSnake(r *Router) *virtualSnake {
	snake := &virtualSnake{
		r:      r,
		_table: make(virtualSnakeTable),
	}
	snake.bootstrap = time.AfterFunc(time.Second, snake.bootstrapNow)
	snake.bootstrap.Stop()
	go snake.maintain()
	return snake
}

func (t *virtualSnake) ascending() *virtualSnakeNeighbour {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t._ascending
}

func (t *virtualSnake) descending() *virtualSnakeNeighbour {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t._descending
}

// maintain will run continuously on a given interval between
// every 1 second and virtualSnakeSetupInterval seconds, sending
// bootstraps and setup messages as needed.
func (t *virtualSnake) maintain() {
	for {
		select {
		case <-t.r.context.Done():
			return
		case <-time.After(time.Second):
		}

		rootKey := t.r.RootPublicKey()
		canBootstrap := rootKey != t.r.public && t.r.PeerCount(-1) > 0
		willBootstrap := false

		if asc := t.ascending(); asc != nil {
			switch {
			case time.Since(asc.LastSeen) > virtualSnakeNeighExpiryPeriod:
				t.teardownPath(asc.PublicKey, asc.PathID, asc.Port, true, fmt.Errorf("ascending neighbour expired"))
				willBootstrap = canBootstrap
			case asc.RootPublicKey != rootKey:
				t.teardownPath(asc.PublicKey, asc.PathID, asc.Port, true, fmt.Errorf("ascending root changed"))
				willBootstrap = canBootstrap
			}
		} else {
			willBootstrap = canBootstrap
		}

		if desc := t.descending(); desc != nil {
			switch {
			case time.Since(desc.LastSeen) > virtualSnakeNeighExpiryPeriod:
				t.teardownPath(desc.PublicKey, desc.PathID, desc.Port, false, fmt.Errorf("descending neighbour expired"))
			case !desc.RootPublicKey.EqualTo(rootKey):
				t.teardownPath(desc.PublicKey, desc.PathID, desc.Port, false, fmt.Errorf("descending root changed"))
			}
		}

		// Send bootstrap messages into the network. Ordinarily we
		// would only want to do this when starting up or after a
		// predefined interval, but for now we'll continue to send
		// them on a regular interval until we can derive some better
		// connection state.
		if willBootstrap {
			t.bootstrapIn(0)
		}
	}
}

func (t *virtualSnake) bootstrapIn(d time.Duration) {
	t.bootstrap.Reset(d)
}

func (t *virtualSnake) bootstrapNow() {
	if t.r.IsRoot() || t.r.PeerCount(-1) == 0 {
		return
	}
	ts, err := util.SignedTimestamp(t.r.private)
	if err != nil {
		return
	}
	var payload [8 + ed25519.PublicKeySize]byte
	bootstrap := types.VirtualSnakeBootstrap{
		RootPublicKey: t.r.RootPublicKey(),
	} // nolint:gosimple
	if _, err := rand.Read(bootstrap.PathID[:]); err != nil {
		return
	}
	if _, err := bootstrap.MarshalBinary(payload[:]); err != nil {
		return
	}

	frame := types.GetFrame()
	frame.Type = types.TypeVirtualSnakeBootstrap
	frame.DestinationKey = t.r.PublicKey()
	frame.Source = t.r.Coords()
	frame.Payload = append(frame.Payload[:0], append(payload[:], ts...)...)
	t.r.send <- frame
}

func (t *virtualSnake) rootNodeChanged(root types.PublicKey) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if asc := t._ascending; asc != nil && !asc.RootPublicKey.EqualTo(root) {
		t.teardownPath(asc.PublicKey, asc.PathID, asc.Port, true, fmt.Errorf("root changed and asc no longer matches"))
		defer t.bootstrapIn(time.Second)
	}
	if desc := t._descending; desc != nil && !desc.RootPublicKey.EqualTo(root) {
		t.teardownPath(desc.PublicKey, desc.PathID, desc.Port, false, fmt.Errorf("root changed and desc no longer matches"))
	}
	teardown := map[virtualSnakeIndex]virtualSnakeEntry{}
	for k, v := range t._table {
		if !root.EqualTo(v.RootPublicKey) {
			teardown[k] = v
		}
	}
	for k := range teardown {
		t.teardownPath(k.PublicKey, k.PathID, 0, false, fmt.Errorf("root or parent changed and tearing paths"))
	}
}

// portWasDisconnected is called by the router when a peer disconnects
// allowing us to clean up the virtual snake state.
func (t *virtualSnake) portWasDisconnected(port types.SwitchPortID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	// If there are no more peers left then clear all state. There's no
	// one to tell, so we don't need to bother sending teardowns.
	if t.r.PeerCount(-1) == 0 {
		t._ascending = nil
		t._descending = nil
		for k := range t._table {
			delete(t._table, k)
		}
		return
	}

	// Check if our ascending or descending directions are affected by
	// this port change.
	if asc := t._ascending; asc != nil && asc.Port == port {
		t.teardownPath(asc.PublicKey, asc.PathID, asc.Port, true, fmt.Errorf("port teardown"))
		defer t.bootstrapIn(0)
	}
	if desc := t._descending; desc != nil && desc.Port == port {
		t.teardownPath(desc.PublicKey, desc.PathID, desc.Port, false, fmt.Errorf("port teardown"))
	}

	// Check if any of our routing table entries are
	// via the port in question. If they were then let's nuke those
	// too, otherwise we'll be trying to route traffic into black
	// holes.
	teardown := map[virtualSnakeIndex]virtualSnakeEntry{}
	for k, v := range t._table {
		if v.DestinationPort == port || v.SourcePort == port {
			teardown[k] = v
		}
	}
	for k := range teardown {
		t.teardownPath(k.PublicKey, k.PathID, 0, false, fmt.Errorf("port teardown"))
	}
}

// getVirtualSnakeNextHop will return the most relevant port
// for a given destination public key.
func (t *virtualSnake) getVirtualSnakeNextHop(from *Peer, destKey types.PublicKey, bootstrap bool) types.SwitchPorts {
	if !bootstrap && t.r.public.EqualTo(destKey) {
		return types.SwitchPorts{0}
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	rootKey := t.r.RootPublicKey()
	ancestors, parentPort := t.r.tree.Ancestors()
	if len(ancestors) > 0 {
		rootKey = ancestors[0]
		ancestors = ancestors[1:]
	}
	bestKey, bestPort := t.r.public, types.SwitchPortID(0) // rootKey, rootPort
	var candidates types.SwitchPorts
	var canlength int
	if !bootstrap {
		candidates, canlength = make(types.SwitchPorts, PortCount), PortCount
	}
	newCandidate := func(key types.PublicKey, port types.SwitchPortID) bool {
		if !bootstrap && port != bestPort {
			canlength--
			candidates[canlength] = port
		}
		bestKey, bestPort = key, port
		return true
	}
	newCheckedCandidate := func(candidate types.PublicKey, port types.SwitchPortID) bool {
		switch {
		case !bootstrap && candidate.EqualTo(destKey) && !bestKey.EqualTo(destKey):
			return newCandidate(candidate, port)
		case util.DHTOrdered(destKey, candidate, bestKey):
			return newCandidate(candidate, port)
		}
		return false
	}

	// Check if we can use the path to the root as a starting point
	switch {
	case bootstrap && bestKey.EqualTo(destKey):
		// Bootstraps always start working towards the root so that
		// they go somewhere rather than getting stuck
		newCandidate(rootKey, parentPort)
	case destKey.EqualTo(rootKey):
		// The destination is actually the root node itself
		newCandidate(rootKey, parentPort)
	case util.DHTOrdered(bestKey, destKey, rootKey):
		// The destination key is higher than our own key, so
		// start using the path to the root as the first candidate
		newCandidate(rootKey, parentPort)
	}

	// Check our direct ancestors
	// bestKey <= destKey < rootKey
	for _, ancestor := range ancestors {
		newCheckedCandidate(ancestor, parentPort)
	}

	// The next section needs us to check direct peers
	activePeers := map[*Peer]*rootAnnouncementWithTime{}
	for _, peer := range t.r.activePorts() {
		ann := peer.lastAnnouncement()
		if ann == nil || ann.RootPublicKey != rootKey {
			continue
		}
		activePeers[peer] = ann
		if bootstrap {
			newCheckedCandidate(peer.public, peer.port)
		}
	}

	// Check our direct peers ancestors
	for peer, peerAnn := range activePeers {
		newCheckedCandidate(peerAnn.RootPublicKey, peer.port)
		for _, hop := range peerAnn.Signatures {
			newCheckedCandidate(hop.PublicKey, peer.port)
		}
	}

	// Check our DHT entries
	for dhtKey, entry := range t._table {
		switch {
		case !entry.Valid():
			continue
		default:
			newCheckedCandidate(dhtKey.PublicKey, entry.SourcePort)
		}
	}

	// Check our direct peers
	for peer := range activePeers {
		if peerKey := peer.PublicKey(); bestKey.EqualTo(peerKey) {
			// We've seen this key already, either as one of our ancestors
			// or as an ancestor of one of our peers, but it turns out we
			// are directly peered with that node, so use the more direct
			// path instead
			newCandidate(peerKey, peer.port)
		}
	}

	if bootstrap {
		return types.SwitchPorts{bestPort}
	} else {
		if canlength == PortCount && t.r.imprecise.Load() {
			return types.SwitchPorts{0}
		}
		return candidates[canlength:]
	}
}

func (t *virtualSnake) handleVirtualSnakeTeardownFromNetwork(from *Peer, rx *types.Frame) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	defer rx.Done()
	if len(rx.Payload) < 8 {
		return
	}
	var teardown types.VirtualSnakeTeardown
	if _, err := teardown.UnmarshalBinary(rx.Payload); err != nil {
		return
	}
	for _, via := range t.handleVirtualSnakeTeardown(from, rx.DestinationKey, teardown.PathID) {
		if t.r.ports[via].started.Load() {
			t.r.ports[via].protoOut.push(rx.Borrow())
		}
	}
}

func (t *virtualSnake) handleVirtualSnakeTeardown(from *Peer, pathKey types.PublicKey, pathID types.VirtualSnakePathID) types.SwitchPorts {
	if desc := t._descending; desc != nil && desc.PublicKey.EqualTo(pathKey) && desc.PathID == pathID {
		t._descending = nil
	}
	if asc := t._ascending; asc != nil && t.r.public.EqualTo(pathKey) && asc.PathID == pathID {
		t._ascending = nil
		defer t.bootstrapIn(0)
	}
	for k, v := range t._table {
		if k.PublicKey == pathKey && k.PathID == pathID {
			delete(t._table, k)
			switch {
			case from == nil: // the teardown originated locally
				if v.DestinationPort != 0 && v.SourcePort != 0 {
					return types.SwitchPorts{v.DestinationPort, v.SourcePort}
				} else if v.DestinationPort != 0 {
					return types.SwitchPorts{v.DestinationPort}
				} else if v.SourcePort != 0 {
					return types.SwitchPorts{v.SourcePort}
				}
			case from.port == v.DestinationPort:
				return types.SwitchPorts{v.SourcePort}
			case from.port == v.SourcePort:
				return types.SwitchPorts{v.DestinationPort}
			}
		}
	}
	return types.SwitchPorts{}
}

func (t *virtualSnake) teardownPath(pathKey types.PublicKey, pathID types.VirtualSnakePathID, via types.SwitchPortID, asc bool, err error) {
	var payload [9]byte
	teardown := types.VirtualSnakeTeardown{ // nolint:gosimple
		PathID: pathID,
	}
	if _, err := teardown.MarshalBinary(payload[:]); err != nil {
		return
	}
	frame := types.GetFrame()
	defer frame.Done()
	frame.Type = types.TypeVirtualSnakeTeardown
	frame.SourceKey = t.r.public
	if asc {
		// If it's an ascending path then the path will have our key
		// in it, not the destination key, so we should make sure we
		// set that in the teardown properly.
		frame.DestinationKey = t.r.public
	} else {
		frame.DestinationKey = pathKey
	}
	frame.Payload = append(frame.Payload[:0], payload[:]...)
	nexthops := t.handleVirtualSnakeTeardown(nil, pathKey, pathID)
	if via != 0 && t.r.ports[via].started.Load() {
		t.r.ports[via].protoOut.push(frame.Borrow())
	} else if via == 0 {
		for _, via := range nexthops {
			if t.r.ports[via].started.Load() {
				t.r.ports[via].protoOut.push(frame.Borrow())
			}
		}
	}
}

// handleBootstrap is called in response to an incoming bootstrap
// packet.
func (t *virtualSnake) handleBootstrap(from *Peer, rx *types.Frame) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if rx.DestinationKey.EqualTo(t.r.public) {
		return nil
	}
	// Unmarshal the bootstrap.
	var bootstrap types.VirtualSnakeBootstrap
	n, err := bootstrap.UnmarshalBinary(rx.Payload)
	if err != nil {
		return fmt.Errorf("bootstrap.UnmarshalBinary: %w", err)
	}
	// Check if the packet has a valid signed timestamp.
	if !util.VerifySignedTimestamp(rx.DestinationKey, rx.Payload[n:]) {
		return fmt.Errorf("util.VerifySignedTimestamp")
	}
	if !bootstrap.RootPublicKey.EqualTo(t.r.RootPublicKey()) {
		return fmt.Errorf("bootstrap root key doesn't match")
	}
	acknowledge := false
	desc := t._descending
	switch {
	case rx.SourceKey.EqualTo(t.r.public):
		// We received a bootstrap from ourselves. This shouldn't happen,
		// so either another node has forwarded it to us incorrectly, or
		// a routing loop has occurred somewhere. Don't act on the bootstrap
		// in that case.
	case desc != nil && desc.PublicKey.EqualTo(rx.DestinationKey):
		// We've received another bootstrap from our direct descending node.
		// Send back an acknowledgement as this is OK.
		acknowledge = true
	case desc != nil && time.Since(desc.LastSeen) >= virtualSnakeNeighExpiryPeriod:
		// We already have a direct descending node, but we haven't seen it
		// recently, so it's quite possible that it has disappeared. We'll
		// therefore handle this bootstrap instead. If the original node comes
		// back later and is closer to us then we'll end up using it again.
		acknowledge = true
	case desc == nil && util.LessThan(rx.DestinationKey, t.r.public):
		// We don't know about a descending node and at the moment we don't know
		// any better candidates, so we'll accept a bootstrap from a node with a
		// key lower than ours (so that it matches descending order).
		acknowledge = true
	case desc != nil && util.DHTOrdered(desc.PublicKey, rx.DestinationKey, t.r.public):
		// We know about a descending node already but it turns out that this
		// new node that we've received a bootstrap from is actually closer to
		// us than the previous node. We'll update our record to use the new
		// node instead and then send back a bootstrap ACK.
		acknowledge = true
	default:
		// The bootstrap conditions weren't met. This might just be because
		// there's a node out there that hasn't converged to a closer node
		// yet, so we'll just ignore the bootstrap.
	}
	if acknowledge {
		bootstrapACK := types.VirtualSnakeBootstrapACK{ // nolint:gosimple
			PathID:        bootstrap.PathID,
			RootPublicKey: bootstrap.RootPublicKey,
		}
		var buf [8 + ed25519.PublicKeySize]byte
		if _, err := bootstrapACK.MarshalBinary(buf[:]); err != nil {
			return fmt.Errorf("bootstrapACK.MarshalBinary: %w", err)
		}
		ts, err := util.SignedTimestamp(t.r.private)
		if err != nil {
			return fmt.Errorf("util.SignedTimestamp: %w", err)
		}
		frame := types.GetFrame()
		frame.Type = types.TypeVirtualSnakeBootstrapACK
		frame.Destination = rx.Source
		frame.DestinationKey = rx.DestinationKey
		frame.Source = t.r.Coords()
		frame.SourceKey = t.r.public
		frame.Payload = append(frame.Payload[:0], append(buf[:], ts...)...)
		t.r.send <- frame
	}
	return nil
}

func (t *virtualSnake) handleBootstrapACK(from *Peer, rx *types.Frame) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	// Unmarshal the bootstrap ACK.
	var bootstrapACK types.VirtualSnakeBootstrapACK
	n, err := bootstrapACK.UnmarshalBinary(rx.Payload)
	if err != nil {
		return fmt.Errorf("bootstrapACK.UnmarshalBinary: %w", err)
	}
	// Check if the packet has a valid signed timestamp.
	if !util.VerifySignedTimestamp(rx.SourceKey, rx.Payload[n:]) {
		return fmt.Errorf("util.VerifySignedTimestamp")
	}
	if !bootstrapACK.RootPublicKey.EqualTo(t.r.RootPublicKey()) {
		return fmt.Errorf("bootstrap ACK root key doesn't match")
	}
	update := false
	asc := t._ascending
	switch {
	case rx.SourceKey.EqualTo(t.r.public):
		// We received a bootstrap ACK from ourselves. This shouldn't happen,
		// so either another node has forwarded it to us incorrectly, or
		// a routing loop has occurred somewhere. Don't act on the bootstrap
		// in that case.
	case asc != nil && asc.PublicKey.EqualTo(rx.SourceKey) && asc.PathID != bootstrapACK.PathID:
		// We've received another bootstrap ACK from our direct ascending node.
		// Just refresh the record and then send a new path setup message to
		// that node.
		update = true
	case asc != nil && time.Since(asc.LastSeen) >= virtualSnakeNeighExpiryPeriod:
		// We already have a direct ascending node, but we haven't seen it
		// recently, so it's quite possible that it has disappeared. We'll
		// therefore handle this bootstrap ACK instead. If the original node comes
		// back later and is closer to us then we'll end up using it again.
		update = true
	case asc == nil && util.LessThan(t.r.public, rx.SourceKey):
		// We don't know about an ascending node and at the moment we don't know
		// any better candidates, so we'll accept a bootstrap ACK from a node with a
		// key higher than ours (so that it matches descending order).
		update = true
	case asc != nil && util.DHTOrdered(t.r.public, rx.SourceKey, asc.PublicKey):
		// We know about an ascending node already but it turns out that this
		// new node that we've received a bootstrap from is actually closer to
		// us than the previous node. We'll update our record to use the new
		// node instead and then send a new path setup message to it.
		update = true
	default:
		// The bootstrap ACK conditions weren't met. This might just be because
		// there's a node out there that hasn't converged to a closer node
		// yet, so we'll just ignore the acknowledgement.
	}
	if update {
		if asc != nil {
			// Tear down the previous path, if there was one.
			t.teardownPath(asc.PublicKey, asc.PathID, asc.Port, true, fmt.Errorf("replacing ascending"))
		}
		t._ascending = &virtualSnakeNeighbour{
			PublicKey:     rx.SourceKey,
			Port:          from.port,
			LastSeen:      time.Now(),
			Coords:        rx.Source,
			PathID:        bootstrapACK.PathID,
			RootPublicKey: bootstrapACK.RootPublicKey,
		}
		setup := types.VirtualSnakeSetup{ // nolint:gosimple
			PathID:        bootstrapACK.PathID,
			RootPublicKey: bootstrapACK.RootPublicKey,
		}
		var buf [8 + ed25519.PublicKeySize]byte
		if _, err := setup.MarshalBinary(buf[:]); err != nil {
			return fmt.Errorf("setup.MarshalBinary: %w", err)
		}
		ts, err := util.SignedTimestamp(t.r.private)
		if err != nil {
			return fmt.Errorf("util.SignedTimestamp: %w", err)
		}

		frame := types.GetFrame()
		frame.Type = types.TypeVirtualSnakeSetup
		frame.Destination = rx.Source
		frame.DestinationKey = rx.SourceKey
		frame.SourceKey = t.r.public
		frame.Payload = append(frame.Payload[:0], append(buf[:], ts...)...)
		t.r.send <- frame
	}
	return nil
}

// handleSetup is called in response to an incoming path setup packet. Note
// that the setup packet isn't necessarily destined for us directly, but is
// instead called for every setup packet being transited through this node.
// It will update the routing table with the new path.
func (t *virtualSnake) handleSetup(from *Peer, rx *types.Frame, nextHops types.SwitchPorts) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	// Unmarshal the setup.
	var setup types.VirtualSnakeSetup
	n, err := setup.UnmarshalBinary(rx.Payload)
	if err != nil {
		return fmt.Errorf("setup.UnmarshalBinary: %w", err)
	}

	// Check if the setup packet has a valid signed timestamp.
	if !util.VerifySignedTimestamp(rx.SourceKey, rx.Payload[n:]) {
		t.teardownPath(rx.SourceKey, setup.PathID, from.port, false, fmt.Errorf("rejecting setup (invalid signature)"))
		return fmt.Errorf("invalid signature")
	}
	if !setup.RootPublicKey.EqualTo(t.r.RootPublicKey()) {
		t.teardownPath(rx.SourceKey, setup.PathID, from.port, false, fmt.Errorf("rejecting setup (root key doesn't match)"))
		return fmt.Errorf("setup root key doesn't match")
	}

	// Did the setup hit a dead end on the way to the ascending node?
	if nextHops.EqualTo(types.SwitchPorts{0}) || nextHops.EqualTo(types.SwitchPorts{}) {
		if !rx.DestinationKey.EqualTo(t.r.public) {
			t.teardownPath(rx.SourceKey, setup.PathID, from.port, false, fmt.Errorf("rejecting setup (hit dead end)"))
			return fmt.Errorf("setup for %q (%s) en route to %q %s hit dead end at %s", rx.SourceKey, hex.EncodeToString(setup.PathID[:]), rx.DestinationKey, rx.Destination, t.r.Coords())
		}
	}

	var addToRoutingTable bool

	// Is the setup a duplicate of one we already have in our table?
	_, ok := t._table[virtualSnakeIndex{rx.SourceKey, setup.PathID}]
	if ok {
		t.teardownPath(rx.SourceKey, setup.PathID, 0, false, fmt.Errorf("rejecting setup (duplicate)"))
		t.teardownPath(rx.SourceKey, setup.PathID, from.port, false, fmt.Errorf("rejecting setup (duplicate)"))
		return fmt.Errorf("setup is a duplicate")
	}

	// If we're at the destination of the setup then update our predecessor
	// with information from the bootstrap.
	if rx.DestinationKey.EqualTo(t.r.public) {
		update := false
		desc := t._descending
		switch {
		case rx.SourceKey.EqualTo(t.r.public):
			// We received a bootstrap from ourselves. This shouldn't happen,
			// so either another node has forwarded it to us incorrectly, or
			// a routing loop has occurred somewhere. Don't act on the bootstrap
			// in that case.
		case desc != nil && desc.PublicKey.EqualTo(rx.SourceKey):
			// We've received another bootstrap from our direct descending node.
			// Just refresh the record and then send back an acknowledgement.
			update = true
		case desc != nil && time.Since(desc.LastSeen) >= virtualSnakeNeighExpiryPeriod:
			// We already have a direct descending node, but we haven't seen it
			// recently, so it's quite possible that it has disappeared. We'll
			// therefore handle this bootstrap instead. If the original node comes
			// back later and is closer to us then we'll end up using it again.
			update = true
		case desc == nil && util.LessThan(rx.SourceKey, t.r.public):
			// We don't know about a descending node and at the moment we don't know
			// any better candidates, so we'll accept a bootstrap from a node with a
			// key lower than ours (so that it matches descending order).
			update = true
		case desc != nil && util.DHTOrdered(desc.PublicKey, rx.SourceKey, t.r.public):
			// We know about a descending node already but it turns out that this
			// new node that we've received a bootstrap from is actually closer to
			// us than the previous node. We'll update our record to use the new
			// node instead and then send back a bootstrap ACK.
			update = true
		default:
			// The bootstrap conditions weren't met. This might just be because
			// there's a node out there that hasn't converged to a closer node
			// yet, so we'll just ignore the bootstrap.
		}
		if update {
			if desc != nil {
				// Tear down the previous path, if there was one.
				t.teardownPath(desc.PublicKey, desc.PathID, desc.Port, false, fmt.Errorf("replacing descending"))
			}
			t._descending = &virtualSnakeNeighbour{
				PublicKey:     rx.SourceKey,
				Port:          from.port,
				LastSeen:      time.Now(),
				Coords:        rx.Source,
				PathID:        setup.PathID,
				RootPublicKey: setup.RootPublicKey,
			}
			addToRoutingTable = true
		}
	} else {
		addToRoutingTable = true
	}

	if addToRoutingTable {
		// Add a new routing table entry.
		index := virtualSnakeIndex{
			PublicKey: rx.SourceKey,
			PathID:    setup.PathID,
		}
		entry := virtualSnakeEntry{
			LastSeen:      time.Now(),
			SourcePort:    from.port,
			RootPublicKey: setup.RootPublicKey,
		}
		if len(nextHops) > 0 {
			entry.DestinationPort = nextHops[0]
		}
		t._table[index] = entry

		return nil
	}

	t.teardownPath(rx.SourceKey, setup.PathID, from.port, false, fmt.Errorf("rejecting setup (no conditions met)"))
	return nil
}
