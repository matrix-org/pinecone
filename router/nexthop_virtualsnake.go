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
	"sync"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
	"go.uber.org/atomic"
)

const virtualSnakeSetupInterval = time.Second * 12
const virtualSnakeNeighExpiryPeriod = time.Minute * 10

type virtualSnake struct {
	r                *Router
	maintainInterval atomic.Int32
	maintainNow      util.Dispatch
	table            virtualSnakeTable
	tableMutex       sync.RWMutex
	ascending        *virtualSnakeNeighbour
	ascendingMutex   sync.RWMutex
	descending       *virtualSnakeNeighbour
	descendingMutex  sync.RWMutex
}

type virtualSnakeIndex struct {
	PublicKey types.PublicKey
	PathID    types.VirtualSnakePathID
}

type virtualSnakeTable map[virtualSnakeIndex]virtualSnakeEntry

type virtualSnakeEntry struct {
	SourcePort      types.SwitchPortID
	DestinationPort types.SwitchPortID
	LastSeen        time.Time
}

func (e *virtualSnakeEntry) Valid() bool {
	// TODO: this should not be needed, since we should only remove
	// paths by teardown ideally
	return true
	//return time.Since(e.LastSeen) < (virtualSnakeSetupInterval * 2)
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
		r:           r,
		maintainNow: util.NewDispatch(),
		table:       make(virtualSnakeTable),
	}
	go snake.maintain()
	return snake
}

// maintain will run continuously on a given interval between
// every 1 second and virtualSnakeSetupInterval seconds, sending
// bootstraps and setup messages as needed.
func (v *virtualSnake) maintain() {
	for {
		peerCount := v.r.PeerCount(-1)
		if peerCount == 0 {
			v.maintainInterval.Store(1)
		}
		exp := v.maintainInterval.Load() + 1
		after := time.Second * time.Duration(exp)
		select {
		case <-v.r.context.Done():
			return
		case <-time.After(after):
		case <-v.maintainNow:
		}
		if peerCount == 0 {
			// If there are no peers connected then we don't need
			// to do any hard maintenance work.
			continue
		}
		if after < virtualSnakeSetupInterval {
			v.maintainInterval.Inc()
		}

		v.ascendingMutex.RLock()
		ascending := v.ascending
		v.ascendingMutex.RUnlock()

		v.descendingMutex.RLock()
		descending := v.descending
		v.descendingMutex.RUnlock()

		if ascending != nil {
			switch {
			case time.Since(ascending.LastSeen) > virtualSnakeNeighExpiryPeriod:
				fallthrough
			case ascending.RootPublicKey != v.r.RootPublicKey():
				v.clearRoutingEntriesForPublicKey(ascending.PublicKey, ascending.PathID, true)
				ascending = nil
			}
		}

		if descending != nil {
			switch {
			case time.Since(descending.LastSeen) > virtualSnakeNeighExpiryPeriod:
				fallthrough
			case descending.RootPublicKey != v.r.RootPublicKey():
				v.clearRoutingEntriesForPublicKey(descending.PublicKey, descending.PathID, false)
				descending = nil
			}
		}

		// Send bootstrap messages into the network. Ordinarily we
		// would only want to do this when starting up or after a
		// predefined interval, but for now we'll continue to send
		// them on a regular interval until we can derive some better
		// connection state.
		switch {
		case ascending == nil || descending == nil:
			v.maintainInterval.Store(0)
			fallthrough
		default:
			if !v.r.IsRoot() { // check we aren't the root
				ts, err := util.SignedTimestamp(v.r.private)
				if err != nil {
					return
				}
				var payload [8 + ed25519.PublicKeySize]byte
				bootstrap := types.VirtualSnakeBootstrap{
					RootPublicKey: v.r.RootPublicKey(),
				} // nolint:gosimple
				if _, err := rand.Read(bootstrap.PathID[:]); err != nil {
					return
				}
				if _, err := bootstrap.MarshalBinary(payload[:]); err != nil {
					return
				}
				v.r.send <- types.Frame{
					Type:           types.TypeVirtualSnakeBootstrap,
					DestinationKey: v.r.PublicKey(), // routes using keys
					Source:         v.r.Coords(),    // used to send back a setup using ygg greedy routing
					Payload:        append(payload[:], ts...),
				}
			}
		}
	}
}

// portWasDisconnected is called by the router when a peer connects
// allowing us to start a new bootstrap.
func (t *virtualSnake) rootNodeChanged(root types.PublicKey) {
	t.maintainNow.Dispatch()
}

// portWasDisconnected is called by the router when a peer connects
// allowing us to start a new bootstrap.
func (t *virtualSnake) portWasConnected(port types.SwitchPortID) {
	t.maintainNow.Dispatch()
}

// portWasDisconnected is called by the router when a peer disconnects
// allowing us to clean up the virtual snake state.
func (t *virtualSnake) portWasDisconnected(port types.SwitchPortID) {
	// If there are no more peers left then clear all state.
	if t.r.PeerCount(-1) == 0 {
		t.ascendingMutex.Lock()
		t.ascending = nil
		t.ascendingMutex.Unlock()
		t.descendingMutex.Lock()
		t.descending = nil
		t.descendingMutex.Unlock()
		t.tableMutex.Lock()
		for k := range t.table {
			delete(t.table, k)
		}
		t.tableMutex.Unlock()
		t.maintainInterval.Store(0)
		return
	}

	defer t.maintainNow.Dispatch()
	// Check if any of our routing table entries are
	// via the port in question. If they were then let's nuke those
	// too, otherwise we'll be trying to route traffic into black
	// holes.
	t.clearRoutingEntriesForPort(port)
}

// getVirtualSnakeNextHop will return the most relevant port
// for a given destination public key.
func (t *virtualSnake) getVirtualSnakeNextHop(from *Peer, destKey types.PublicKey, bootstrap bool) types.SwitchPorts {
	if !bootstrap && t.r.public.EqualTo(destKey) {
		return types.SwitchPorts{0}
	}
	rootKey := t.r.public
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

	// Check our direct peers ancestors
	for _, peer := range t.r.activePorts() {
		peerAnn := peer.lastAnnouncement()
		if peerAnn == nil {
			continue
		}
		newCheckedCandidate(peerAnn.RootPublicKey, peer.port)
		for _, hop := range peerAnn.Signatures {
			newCheckedCandidate(hop.PublicKey, peer.port)
		}
	}

	// Check our direct peers
	for _, peer := range t.r.activePorts() {
		peerKey := peer.PublicKey()
		switch {
		case bestKey.EqualTo(peerKey):
			// We've seen this key already, either as one of our ancestors
			// or as an ancestor of one of our peers, but it turns out we
			// are directly peered with that node, so use the more direct
			// path instead
			newCandidate(peerKey, peer.port)
		}
	}

	// Check our DHT entries
	t.tableMutex.RLock()
	for dhtKey, entry := range t.table {
		switch {
		case !entry.Valid():
			continue
		}
		newCheckedCandidate(dhtKey.PublicKey, entry.SourcePort)
	}
	t.tableMutex.RUnlock()

	if bootstrap {
		return types.SwitchPorts{bestPort}
	} else {
		if canlength == PortCount && t.r.imprecise.Load() {
			return types.SwitchPorts{0}
		}
		return candidates[canlength:]
	}
}

func (t *virtualSnake) getVirtualSnakeTeardownNextHop(from *Peer, rx *types.Frame) types.SwitchPorts {
	if len(rx.Payload) < 1 {
		return types.SwitchPorts{}
	}
	// Unmarshal the bootstrap.
	var teardown types.VirtualSnakeTeardown
	if _, err := teardown.UnmarshalBinary(rx.Payload); err != nil {
		return types.SwitchPorts{}
	}
	changed := false
	if teardown.Ascending {
		t.descendingMutex.Lock()
		if desc := t.descending; desc != nil && desc.PublicKey == rx.DestinationKey {
			t.descending, changed = nil, true
		}
		t.descendingMutex.Unlock()
	} else {
		t.ascendingMutex.Lock()
		if asc := t.ascending; asc != nil && asc.PublicKey == rx.DestinationKey {
			t.ascending, changed = nil, true
		}
		t.ascendingMutex.Unlock()
	}
	if changed {
		defer t.maintainNow.Dispatch()
	}
	t.tableMutex.Lock()
	defer t.tableMutex.Unlock()
	for k, v := range t.table {
		if k.PublicKey == rx.DestinationKey && k.PathID == teardown.PathID {
			delete(t.table, k)
			if teardown.Ascending {
				return types.SwitchPorts{v.DestinationPort}
			} else {
				return types.SwitchPorts{v.SourcePort}
			}
		}
	}
	return types.SwitchPorts{}
}

func (t *virtualSnake) clearRoutingEntriesForPort(port types.SwitchPortID) {
	t.tableMutex.RLock()
	asc := map[types.PublicKey]types.VirtualSnakePathID{}
	desc := map[types.PublicKey]types.VirtualSnakePathID{}
	for k, v := range t.table {
		if v.DestinationPort == port {
			asc[k.PublicKey] = k.PathID
		}
		if v.SourcePort == port {
			desc[k.PublicKey] = k.PathID
		}
	}
	t.tableMutex.RUnlock()
	for k, pathID := range asc {
		var payload [9]byte
		teardown := types.VirtualSnakeTeardown{ // nolint:gosimple
			PathID:    pathID,
			Ascending: true,
		}
		if _, err := teardown.MarshalBinary(payload[:]); err != nil {
			return
		}
		t.r.send <- types.Frame{
			Type:           types.TypeVirtualSnakeTeardown,
			SourceKey:      t.r.public,
			DestinationKey: k,
			Payload:        payload[:],
		}
	}
	for k, pathID := range desc {
		var payload [9]byte
		teardown := types.VirtualSnakeTeardown{ // nolint:gosimple
			PathID:    pathID,
			Ascending: false,
		}
		if _, err := teardown.MarshalBinary(payload[:]); err != nil {
			return
		}
		t.r.send <- types.Frame{
			Type:           types.TypeVirtualSnakeTeardown,
			SourceKey:      t.r.public,
			DestinationKey: k,
			Payload:        payload[:],
		}
	}
}

func (t *virtualSnake) clearRoutingEntriesForPublicKey(pk types.PublicKey, pathID types.VirtualSnakePathID, ascending bool) {
	var payload [9]byte
	teardown := types.VirtualSnakeTeardown{ // nolint:gosimple
		PathID:    pathID,
		Ascending: ascending,
	}
	if _, err := teardown.MarshalBinary(payload[:]); err != nil {
		return
	}
	t.r.send <- types.Frame{
		Type:           types.TypeVirtualSnakeTeardown,
		SourceKey:      t.r.public,
		DestinationKey: pk,
		Payload:        payload[:],
	}
}

// handleBootstrap is called in response to an incoming bootstrap
// packet. It will update the descending information and send a setup
// message if needed.
func (t *virtualSnake) handleBootstrap(from *Peer, rx *types.Frame) error {
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
	if bootstrap.RootPublicKey != t.r.RootPublicKey() {
		return fmt.Errorf("root key doesn't match")
	}
	bootstrapACK := types.VirtualSnakeBootstrapACK{ // nolint:gosimple
		PathID:        bootstrap.PathID,
		RootPublicKey: t.r.RootPublicKey(),
	}
	var buf [8 + ed25519.PublicKeySize]byte
	if _, err := bootstrapACK.MarshalBinary(buf[:]); err != nil {
		return fmt.Errorf("bootstrapACK.MarshalBinary: %w", err)
	}
	ts, err := util.SignedTimestamp(t.r.private)
	if err != nil {
		return fmt.Errorf("util.SignedTimestamp: %w", err)
	}
	t.r.send <- types.Frame{
		Destination:    rx.Source,
		DestinationKey: rx.DestinationKey,
		Source:         t.r.Coords(),
		SourceKey:      t.r.PublicKey(),
		Type:           types.TypeVirtualSnakeBootstrapACK,
		Payload:        append(buf[:], ts...),
	}
	return nil
}

func (t *virtualSnake) handleBootstrapACK(from *Peer, rx *types.Frame) error {
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
	var send *types.Frame
	defer func() {
		// When we reach the end of handleBootstrap, if a response packet
		// has been built, then send it into the network. Doing this in a
		// defer means we can defer the mutex unlocks but still not hold
		// them for longer than needed.
		if send != nil {
			t.r.send <- *send
		}
	}()
	t.ascendingMutex.Lock()
	defer t.ascendingMutex.Unlock()
	switch {
	case rx.SourceKey.EqualTo(t.r.public):
		// We received a bootstrap ACK from ourselves. This shouldn't happen,
		// so either another node has forwarded it to us incorrectly, or
		// a routing loop has occurred somewhere. Don't act on the bootstrap
		// in that case.
	case bootstrapACK.RootPublicKey != t.r.RootPublicKey():
		// The root key doesn't match ours so ignore the message
	case t.ascending != nil && t.ascending.PublicKey.EqualTo(rx.SourceKey):
		// We've received another bootstrap ACK from our direct ascending node.
		// Just refresh the record and then send a new path setup message to
		// that node.
		fallthrough
	case t.ascending != nil && time.Since(t.ascending.LastSeen) >= virtualSnakeNeighExpiryPeriod:
		// We already have a direct ascending node, but we haven't seen it
		// recently, so it's quite possible that it has disappeared. We'll
		// therefore handle this bootstrap ACK instead. If the original node comes
		// back later and is closer to us then we'll end up using it again.
		fallthrough
	case t.ascending == nil && util.LessThan(t.r.public, rx.SourceKey):
		// We don't know about an ascending node and at the moment we don't know
		// any better candidates, so we'll accept a bootstrap ACK from a node with a
		// key higher than ours (so that it matches descending order).
		fallthrough
	case t.ascending != nil && util.DHTOrdered(t.r.public, rx.SourceKey, t.ascending.PublicKey):
		// We know about an ascending node already but it turns out that this
		// new node that we've received a bootstrap from is actually closer to
		// us than the previous node. We'll update our record to use the new
		// node instead and then send a new path setup message to it.
		if t.ascending != nil {
			if rx.SourceKey != t.ascending.PublicKey || bootstrapACK.PathID != t.ascending.PathID {
				t.clearRoutingEntriesForPublicKey(t.ascending.PublicKey, t.ascending.PathID, true)
			}
		}
		t.ascending = &virtualSnakeNeighbour{
			PublicKey:     rx.SourceKey,
			Port:          from.port,
			LastSeen:      time.Now(),
			Coords:        rx.Source,
			PathID:        bootstrapACK.PathID,
			RootPublicKey: bootstrapACK.RootPublicKey,
		}
		setup := types.VirtualSnakeSetup{ // nolint:gosimple
			PathID:        bootstrapACK.PathID,
			RootPublicKey: t.r.RootPublicKey(),
		}
		var buf [8 + ed25519.PublicKeySize]byte
		if _, err := setup.MarshalBinary(buf[:]); err != nil {
			return fmt.Errorf("setup.MarshalBinary: %w", err)
		}
		ts, err := util.SignedTimestamp(t.r.private)
		if err != nil {
			return fmt.Errorf("util.SignedTimestamp: %w", err)
		}
		send = &types.Frame{
			Destination:    rx.Source,
			DestinationKey: rx.SourceKey,
			SourceKey:      t.r.public,
			Type:           types.TypeVirtualSnakeSetup,
			Payload:        append(buf[:], ts...),
		}
	default:
		// The bootstrap ACK conditions weren't met. This might just be because
		// there's a node out there that hasn't converged to a closer node
		// yet, so we'll just ignore the acknowledgement.
	}
	return nil
}

// handleSetup is called in response to an incoming path setup packet. Note
// that the setup packet isn't necessarily destined for us directly, but is
// instead called for every setup packet being transited through this node.
// It will update the routing table with the new path.
func (t *virtualSnake) handleSetup(from *Peer, rx *types.Frame, nextHops types.SwitchPorts) error {
	// Unmarshal the setup.
	var setup types.VirtualSnakeSetup
	n, err := setup.UnmarshalBinary(rx.Payload)
	if err != nil {
		return fmt.Errorf("setup.UnmarshalBinary: %w", err)
	}

	// Check if the setup packet has a valid signed timestamp.
	if !util.VerifySignedTimestamp(rx.SourceKey, rx.Payload[n:]) {
		return fmt.Errorf("invalid signature")
	}
	if setup.RootPublicKey != t.r.RootPublicKey() {
		t.clearRoutingEntriesForPublicKey(rx.SourceKey, setup.PathID, false)
		return fmt.Errorf("root key doesn't match")
	}

	// Add a new routing table entry.
	// TODO: The routing table needs to be bounded by size, so that we don't
	// exhaust available system memory trying to maintain network paths. To
	// bound the routing table safely, we may want to make sure that we have
	// a reasonable spread of routes across keyspace so that we don't create
	// any obvious routing holes.
	index := virtualSnakeIndex{
		PublicKey: rx.SourceKey,
		PathID:    setup.PathID,
	}
	entry := virtualSnakeEntry{
		LastSeen:   time.Now(),
		SourcePort: from.port,
	}
	if len(nextHops) > 0 {
		entry.DestinationPort = nextHops[0]
	}
	t.tableMutex.Lock()
	t.table[index] = entry
	t.tableMutex.Unlock()

	// Did the setup hit a dead end on the way to the ascending node?
	if nextHops.EqualTo(types.SwitchPorts{0}) && !rx.DestinationKey.EqualTo(t.r.public) {
		t.clearRoutingEntriesForPublicKey(rx.SourceKey, setup.PathID, false)
		return fmt.Errorf("setup for %q %s hit dead end at %s", rx.DestinationKey, rx.Destination, t.r.Coords())
	}

	// If we're at the destination of the setup then update our predecessor
	// with information from the bootstrap.
	if rx.DestinationKey.EqualTo(t.r.public) {
		t.descendingMutex.Lock()
		defer t.descendingMutex.Unlock()
		switch {
		case rx.SourceKey.EqualTo(t.r.public):
			// We received a bootstrap from ourselves. This shouldn't happen,
			// so either another node has forwarded it to us incorrectly, or
			// a routing loop has occurred somewhere. Don't act on the bootstrap
			// in that case.
		case setup.RootPublicKey != t.r.RootPublicKey():
			// The root key doesn't match ours so ignore the message
		case t.descending != nil && t.descending.PublicKey.EqualTo(rx.SourceKey):
			// We've received another bootstrap from our direct descending node.
			// Just refresh the record and then send back an acknowledgement.
			fallthrough
		case t.descending != nil && time.Since(t.descending.LastSeen) >= virtualSnakeNeighExpiryPeriod:
			// We already have a direct descending node, but we haven't seen it
			// recently, so it's quite possible that it has disappeared. We'll
			// therefore handle this bootstrap instead. If the original node comes
			// back later and is closer to us then we'll end up using it again.
			fallthrough
		case t.descending == nil && util.LessThan(rx.SourceKey, t.r.public):
			// We don't know about a descending node and at the moment we don't know
			// any better candidates, so we'll accept a bootstrap from a node with a
			// key lower than ours (so that it matches descending order).
			fallthrough
		case t.descending != nil && util.DHTOrdered(t.descending.PublicKey, rx.SourceKey, t.r.public):
			// We know about a descending node already but it turns out that this
			// new node that we've received a bootstrap from is actually closer to
			// us than the previous node. We'll update our record to use the new
			// node instead and then send back a bootstrap ACK.
			if t.descending != nil {
				if rx.SourceKey != t.descending.PublicKey || t.descending.PathID != setup.PathID {
					t.clearRoutingEntriesForPublicKey(t.descending.PublicKey, t.descending.PathID, false)
				}
			}
			t.descending = &virtualSnakeNeighbour{
				PublicKey:     rx.SourceKey,
				Port:          from.port,
				LastSeen:      time.Now(),
				Coords:        rx.Source,
				PathID:        setup.PathID,
				RootPublicKey: setup.RootPublicKey,
			}
		default:
			// The bootstrap conditions weren't met. This might just be because
			// there's a node out there that hasn't converged to a closer node
			// yet, so we'll just ignore the bootstrap.
		}
	}

	return nil
}
