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
)

const virtualSnakeNeighExpiryPeriod = time.Minute * 10

type virtualSnake struct {
	r               *Router
	maintainNow     util.Dispatch
	table           virtualSnakeTable
	tableMutex      sync.RWMutex
	ascending       *virtualSnakeNeighbour
	ascendingMutex  sync.RWMutex
	descending      *virtualSnakeNeighbour
	descendingMutex sync.RWMutex
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
		r:     r,
		table: make(virtualSnakeTable),
	}
	go snake.maintain()
	return snake
}

// maintain will run continuously on a given interval between
// every 1 second and virtualSnakeSetupInterval seconds, sending
// bootstraps and setup messages as needed.
func (t *virtualSnake) maintain() {
	for {
		peerCount := t.r.PeerCount(-1)
		bootstrapNow := false
		bootstrapConditionally := false
		select {
		case <-t.r.context.Done():
			return
		case <-time.After(time.Second):
			bootstrapConditionally = true
		case <-t.maintainNow:
			bootstrapNow = true
		}
		if peerCount == 0 {
			// If there are no peers connected then we don't need
			// to do any hard maintenance work.
			continue
		}

		t.ascendingMutex.Lock()
		if t.ascending != nil {
			switch {
			case time.Since(t.ascending.LastSeen) > virtualSnakeNeighExpiryPeriod:
				fallthrough
			case t.ascending.RootPublicKey != t.r.RootPublicKey():
				t.sendTeardownForPath(
					t.ascending.PublicKey, t.ascending.PathID, true, 0,
					fmt.Errorf("ascending node changed root"),
				)
				bootstrapNow = true
				t.ascending = nil
			}
		} else if bootstrapConditionally {
			bootstrapNow = true
		}
		t.ascendingMutex.Unlock()

		t.descendingMutex.Lock()
		if t.descending != nil {
			switch {
			case time.Since(t.descending.LastSeen) > virtualSnakeNeighExpiryPeriod:
				fallthrough
			case !t.descending.RootPublicKey.EqualTo(t.r.RootPublicKey()):
				t.sendTeardownForPath(
					t.descending.PublicKey, t.descending.PathID, false, 0,
					fmt.Errorf("descending node changed root"),
				)
				bootstrapNow = true
				t.descending = nil
			}
		} else if bootstrapConditionally {
			bootstrapNow = true
		}
		t.descendingMutex.Unlock()

		// Send bootstrap messages into the network. Ordinarily we
		// would only want to do this when starting up or after a
		// predefined interval, but for now we'll continue to send
		// them on a regular interval until we can derive some better
		// connection state.
		if bootstrapNow {
			t.bootstrapNow()
		}
	}
}

func (t *virtualSnake) bootstrapNow() {
	if t.r.IsRoot() {
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
	t.r.send <- types.Frame{
		Type:           types.TypeVirtualSnakeBootstrap,
		DestinationKey: t.r.PublicKey(), // routes using keys
		Source:         t.r.Coords(),    // used to send back a setup using ygg greedy routing
		Payload:        append(payload[:], ts...),
	}
}

// portWasDisconnected is called by the router when a peer connects
// allowing us to start a new bootstrap.
func (t *virtualSnake) rootNodeChanged(root types.PublicKey) {
	t.ascendingMutex.Lock()
	if asc := t.ascending; asc != nil && !asc.RootPublicKey.EqualTo(root) {
		t.ascending = nil
		t.ascendingMutex.Unlock()
		t.sendTeardownForPath(
			asc.PublicKey, asc.PathID, true, asc.Port,
			fmt.Errorf("root changed and ascending no longer matches"),
		)
		t.tableMutex.RLock()
		for k, v := range t.table {
			t.sendTeardownForPath(k.PublicKey, k.PathID, true, v.DestinationPort, fmt.Errorf("root changed"))
			delete(t.table, k)
		}
		t.tableMutex.RUnlock()
	} else {
		t.ascendingMutex.Unlock()
	}
	t.descendingMutex.Lock()
	if desc := t.descending; desc != nil && !desc.RootPublicKey.EqualTo(root) {
		t.descending = nil
		t.descendingMutex.Unlock()
		t.sendTeardownForPath(
			desc.PublicKey, desc.PathID, false, desc.Port,
			fmt.Errorf("root changed and descending no longer matches"),
		)
		t.tableMutex.RLock()
		for k, v := range t.table {
			t.sendTeardownForPath(k.PublicKey, k.PathID, false, v.SourcePort, fmt.Errorf("root changed"))
			delete(t.table, k)
		}
		t.tableMutex.RUnlock()
	} else {
		t.descendingMutex.Unlock()
	}
	t.bootstrapNow()
}

// portWasDisconnected is called by the router when a peer connects
// allowing us to start a new bootstrap.
func (t *virtualSnake) portWasConnected(port types.SwitchPortID) {

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
		return
	}

	// Check if any of our routing table entries are
	// via the port in question. If they were then let's nuke those
	// too, otherwise we'll be trying to route traffic into black
	// holes.
	t.sendTeardownsForPort(port)
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
		panic("teardown isn't big enough")
		// return types.SwitchPorts{}
	}
	// Unmarshal the bootstrap.
	var teardown types.VirtualSnakeTeardown
	if _, err := teardown.UnmarshalBinary(rx.Payload); err != nil {
		panic("failed to unmarshal teardown")
		// return types.SwitchPorts{}
	}
	var changed bool
	defer func() {
		if changed {
			t.bootstrapNow()
		}
	}()
	var next types.SwitchPortID
	var ascending bool
	t.tableMutex.Lock()
	defer t.tableMutex.Unlock()
entries:
	for k, v := range t.table {
		if k.PublicKey == rx.DestinationKey && k.PathID == teardown.PathID {
			delete(t.table, k)
			switch {
			case from.port == v.DestinationPort:
				next = v.SourcePort
				ascending = false
				break entries
			case from.port == v.SourcePort:
				next = v.DestinationPort
				ascending = true
				break entries
			case from.port == 0:
				if ascending = teardown.Ascending; ascending {
					next = v.DestinationPort
				} else {
					next = v.SourcePort
				}
				break entries
			default:
				panic(fmt.Sprintf("teardown came from port %d but should have been from %d or %d", from.port, v.SourcePort, v.DestinationPort))
			}
		}
	}
	if ascending {
		t.descendingMutex.Lock()
		if desc := t.descending; desc != nil && desc.PublicKey.EqualTo(rx.DestinationKey) && desc.PathID == teardown.PathID {
			t.descending, changed = nil, true
			t.r.log.Println("Cleaning descending")
		}
		t.descendingMutex.Unlock()
	} else {
		t.ascendingMutex.Lock()
		if asc := t.ascending; asc != nil && asc.PublicKey.EqualTo(rx.DestinationKey) && asc.PathID == teardown.PathID {
			t.ascending, changed = nil, true
			t.r.log.Println("Cleaning ascending")
		}
		t.ascendingMutex.Unlock()
	}
	return types.SwitchPorts{next}
}

func (t *virtualSnake) sendTeardownsForPort(port types.SwitchPortID) {
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

func (t *virtualSnake) sendTeardownForPath(pk types.PublicKey, pathID types.VirtualSnakePathID, ascending bool, via types.SwitchPortID, err error) {
	t.r.log.Println("Tear down", pk, "path", pathID, "because:", err)
	var payload [9]byte
	teardown := types.VirtualSnakeTeardown{ // nolint:gosimple
		PathID:    pathID,
		Ascending: ascending,
	}
	if _, err := teardown.MarshalBinary(payload[:]); err != nil {
		return
	}
	frame := types.Frame{
		Type:           types.TypeVirtualSnakeTeardown,
		SourceKey:      t.r.public,
		DestinationKey: pk,
		Payload:        payload[:],
	}
	go func() {
		if via != 0 {
			t.r.ports[via].protoOut <- frame.Borrow()
		} else {
			t.r.send <- frame
		}
	}()
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
	if !bootstrap.RootPublicKey.EqualTo(t.r.RootPublicKey()) {
		return fmt.Errorf("root key doesn't match")
	}
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
	frame := types.Frame{
		Destination:    rx.Source,
		DestinationKey: rx.DestinationKey,
		Source:         t.r.Coords(),
		SourceKey:      t.r.PublicKey(),
		Type:           types.TypeVirtualSnakeBootstrapACK,
		Payload:        append(buf[:], ts...),
	}
	from.protoOut <- frame.Borrow()
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
	if !bootstrapACK.RootPublicKey.EqualTo(t.r.RootPublicKey()) {
		return fmt.Errorf("root key doesn't match")
	}
	t.ascendingMutex.Lock()
	update := false
	switch {
	case rx.SourceKey.EqualTo(t.r.public):
		// We received a bootstrap ACK from ourselves. This shouldn't happen,
		// so either another node has forwarded it to us incorrectly, or
		// a routing loop has occurred somewhere. Don't act on the bootstrap
		// in that case.
	case t.ascending != nil && t.ascending.PublicKey.EqualTo(rx.SourceKey) && t.ascending.PathID != bootstrapACK.PathID:
		// We've received another bootstrap ACK from our direct ascending node.
		// Just refresh the record and then send a new path setup message to
		// that node.
		update = true
	case t.ascending != nil && time.Since(t.ascending.LastSeen) >= virtualSnakeNeighExpiryPeriod:
		// We already have a direct ascending node, but we haven't seen it
		// recently, so it's quite possible that it has disappeared. We'll
		// therefore handle this bootstrap ACK instead. If the original node comes
		// back later and is closer to us then we'll end up using it again.
		update = true
	case t.ascending == nil && util.LessThan(t.r.public, rx.SourceKey):
		// We don't know about an ascending node and at the moment we don't know
		// any better candidates, so we'll accept a bootstrap ACK from a node with a
		// key higher than ours (so that it matches descending order).
		update = true
	case t.ascending != nil && util.DHTOrdered(t.r.public, rx.SourceKey, t.ascending.PublicKey):
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
		if asc := t.ascending; asc != nil /*&& !rx.SourceKey.EqualTo(asc.PublicKey)*/ {
			t.sendTeardownForPath(
				asc.PublicKey, asc.PathID, true, 0,
				fmt.Errorf("replacing ascending node"),
			)
		}
		t.ascending = &virtualSnakeNeighbour{
			PublicKey:     rx.SourceKey,
			Port:          from.port,
			LastSeen:      time.Now(),
			Coords:        rx.Source,
			PathID:        bootstrapACK.PathID,
			RootPublicKey: bootstrapACK.RootPublicKey,
		}
		t.ascendingMutex.Unlock()
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
		frame := types.Frame{
			Destination:    rx.Source,
			DestinationKey: rx.SourceKey,
			SourceKey:      t.r.PublicKey(),
			Type:           types.TypeVirtualSnakeSetup,
			Payload:        append(buf[:], ts...),
		}
		from.protoOut <- frame.Borrow()
	} else {
		t.ascendingMutex.Unlock()
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
	if !setup.RootPublicKey.EqualTo(t.r.RootPublicKey()) {
		t.sendTeardownForPath(
			rx.SourceKey, setup.PathID, false, from.port,
			fmt.Errorf("rejecting setup (root key doesn't match)"),
		)
		return fmt.Errorf("root key doesn't match")
	}

	// Did the setup hit a dead end on the way to the ascending node?
	if nextHops.EqualTo(types.SwitchPorts{0}) && !rx.DestinationKey.EqualTo(t.r.public) {
		t.sendTeardownForPath(
			rx.SourceKey, setup.PathID, false, from.port,
			fmt.Errorf("rejecting setup (hit dead end)"),
		)
		return fmt.Errorf("setup for %q %s hit dead end at %s", rx.DestinationKey, rx.Destination, t.r.Coords())
	}

	var addToRoutingTable bool

	// If we're at the destination of the setup then update our predecessor
	// with information from the bootstrap.
	if rx.DestinationKey.EqualTo(t.r.public) {
		t.descendingMutex.Lock()
		update := false
		switch {
		case rx.SourceKey.EqualTo(t.r.public):
			// We received a bootstrap from ourselves. This shouldn't happen,
			// so either another node has forwarded it to us incorrectly, or
			// a routing loop has occurred somewhere. Don't act on the bootstrap
			// in that case.
		case t.descending != nil && t.descending.PublicKey.EqualTo(rx.SourceKey) && t.descending.PathID != setup.PathID:
			// We've received another bootstrap from our direct descending node.
			// Just refresh the record and then send back an acknowledgement.
			update = true
		case t.descending != nil && time.Since(t.descending.LastSeen) >= virtualSnakeNeighExpiryPeriod:
			// We already have a direct descending node, but we haven't seen it
			// recently, so it's quite possible that it has disappeared. We'll
			// therefore handle this bootstrap instead. If the original node comes
			// back later and is closer to us then we'll end up using it again.
			update = true
		case t.descending == nil && util.LessThan(rx.SourceKey, t.r.public):
			// We don't know about a descending node and at the moment we don't know
			// any better candidates, so we'll accept a bootstrap from a node with a
			// key lower than ours (so that it matches descending order).
			update = true
		case t.descending != nil && util.DHTOrdered(t.descending.PublicKey, rx.SourceKey, t.r.public):
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
			if desc := t.descending; desc != nil {
				t.sendTeardownForPath(
					desc.PublicKey, desc.PathID, false, 0,
					fmt.Errorf("replacing descending node"),
				)
			}
			t.descending = &virtualSnakeNeighbour{
				PublicKey:     rx.SourceKey,
				Port:          from.port,
				LastSeen:      time.Now(),
				Coords:        rx.Source,
				PathID:        setup.PathID,
				RootPublicKey: setup.RootPublicKey,
			}
			t.descendingMutex.Unlock()
			addToRoutingTable = true
		} else {
			t.descendingMutex.Unlock()
			t.sendTeardownForPath(
				rx.SourceKey, setup.PathID, false, from.port,
				fmt.Errorf("rejecting setup (no conditions met)"),
			)
		}
	} else {
		addToRoutingTable = true
	}

	if addToRoutingTable {
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
	}

	return nil
}
