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
	"math"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
	"go.uber.org/atomic"
)

const virtualSnakeSetupInterval = time.Second * 16
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

type virtualSnakeTable map[types.PublicKey]virtualSnakeEntry

type virtualSnakeEntry struct {
	SourcePort      types.SwitchPortID
	DestinationPort types.SwitchPortID
	LastSeen        time.Time
}

type virtualSnakeNeighbour struct {
	PublicKey types.PublicKey
	Port      types.SwitchPortID
	LastSeen  time.Time
	Coords    types.SwitchPorts
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
			v.maintainInterval.Store(0)
		}
		exp := math.Exp2(float64(v.maintainInterval.Load()))
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
		if ascending != nil && time.Since(ascending.LastSeen) < virtualSnakeNeighExpiryPeriod {
			ascending = nil
		}
		v.ascendingMutex.RUnlock()
		v.descendingMutex.RLock()
		descending := v.descending
		if descending != nil && time.Since(descending.LastSeen) < virtualSnakeNeighExpiryPeriod {
			descending = nil
		}
		v.descendingMutex.RUnlock()

		// Send bootstrap messages into the network. Ordinarily we
		// would only want to do this when starting up or after a
		// predefined interval, but for now we'll continue to send
		// them on a regular interval until we can derive some better
		// connection state.
		switch {
		// TODO: case for when the root has changed?
		// TODO: case for when our parent has changed?
		case ascending == nil || descending == nil:
			v.maintainInterval.Store(0)
			fallthrough
		default:
			if !v.r.PublicKey().EqualTo(v.r.RootPublicKey()) { // check we aren't the root
				ts, err := util.SignedTimestamp(v.r.private)
				if err != nil {
					return
				}
				v.r.send <- types.Frame{
					Type:           types.TypeVirtualSnakeBootstrap,
					DestinationKey: v.r.PublicKey(), // routes using keys
					Source:         v.r.Coords(),    // used to send back a setup using ygg greedy routing
					Payload:        ts,
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
	defer t.maintainNow.Dispatch()
	// Check if any of our routing table entries are
	// via the port in question. If they were then let's nuke those
	// too, otherwise we'll be trying to route traffic into black
	// holes.
	t.clearRoutingEntriesForPort(port)
}

// getVirtualSnakeBootstrapNextHop will return the most relevant port
// for a given destination public key.
func (t *virtualSnake) getVirtualSnakeBootstrapNextHop(from *Peer, destKey types.PublicKey) types.SwitchPorts {
	ancestors, parentPort := t.r.tree.Ancestors()
	rootKey := t.r.RootPublicKey()
	bestKey, bestPort := t.r.public, types.SwitchPortID(0) // rootKey, rootPort
	newCandidate := func(key types.PublicKey, port types.SwitchPortID) {
		bestKey, bestPort = key, port
	}

	// Check our direct ancestors
	// bestKey <= destKey < rootKey
	if !util.LessThan(destKey, bestKey) && util.LessThan(destKey, rootKey) {
		newCandidate(rootKey, parentPort)
		for _, ancestor := range ancestors[1:] {
			if util.DHTOrdered(destKey, ancestor, bestKey) {
				newCandidate(ancestor, parentPort)
			}
		}
	}

	// Check our direct peers ancestors
	for _, peer := range t.r.activePorts() {
		peerAnn := peer.lastAnnouncement()
		if peerAnn == nil {
			continue
		}
		for _, hop := range peerAnn.Signatures {
			switch {
			case util.DHTOrdered(destKey, hop.PublicKey, bestKey):
				newCandidate(hop.PublicKey, peer.port)
			}
		}
	}

	// Check our direct peers
	for _, peer := range t.r.activePorts() {
		peerKey := peer.PublicKey()
		switch {
		case bestKey.EqualTo(peerKey):
			newCandidate(peerKey, peer.port)
		case util.DHTOrdered(destKey, peerKey, bestKey):
			newCandidate(peerKey, peer.port)
		}
	}

	// Check our DHT entries
	t.tableMutex.RLock()
	for dhtKey, entry := range t.table {
		switch {
		case util.DHTOrdered(destKey, dhtKey, bestKey):
			newCandidate(dhtKey, entry.SourcePort)
		}
	}
	t.tableMutex.RUnlock()

	return types.SwitchPorts{bestPort}
}

// getVirtualSnakeBootstrapNextHop will return the most relevant port
// for a given destination public key.
func (t *virtualSnake) getVirtualSnakeNextHop(from *Peer, destKey types.PublicKey) types.SwitchPorts {
	if destKey.EqualTo(t.r.public) {
		return types.SwitchPorts{0}
	}
	rootKey := t.r.RootPublicKey()
	if util.LessThan(rootKey, destKey) {
		// The destination key is higher than the root key - this should
		// be impossible since the root node should be the highest end of
		// the snake. We will therefore just drop the packet.
		return nil
	}
	bestKey, bestPort := t.r.public, types.SwitchPortID(0)
	candidates, canlength := make(types.SwitchPorts, PortCount), PortCount
	newCandidate := func(key types.PublicKey, port types.SwitchPortID) {
		if port != bestPort {
			canlength--
			candidates[canlength] = port
		}
		bestKey, bestPort = key, port
	}
	// Start by looking through all ancestors. This includes the root
	// node. These routes are effectively ascending paths to higher
	// public keys.
	if ancestors, rootPort := t.r.tree.Ancestors(); rootPort != 0 {
		for _, ancestorKey := range ancestors {
			switch {
			case util.DHTOrdered(bestKey, ancestorKey, destKey) || ancestorKey.EqualTo(destKey):
				newCandidate(ancestorKey, rootPort)
			case !util.LessThan(destKey, bestKey) && util.LessThan(destKey, ancestorKey):
				newCandidate(ancestorKey, rootPort)
			}
		}
	}
	t.tableMutex.RLock()
	// Next, look at the routing table. This includes source-side paths
	// from setups only.
	for dhtKey, entry := range t.table {
		switch {
		case destKey.EqualTo(dhtKey):
			newCandidate(dhtKey, entry.SourcePort)
		case util.DHTOrdered(destKey, dhtKey, bestKey):
			newCandidate(dhtKey, entry.SourcePort)
		}
	}
	t.tableMutex.RUnlock()
	for _, peer := range t.r.activePorts() {
		// Finally, look at our direct peers, in case of any of our direct
		// peers end up being a better path than anything else we've come up
		// with so far.
		peer.mutex.RLock()
		peerKey := peer.public
		peer.mutex.RUnlock()
		switch {
		case peerKey.EqualTo(destKey):
			newCandidate(peerKey, peer.port)
		case util.DHTOrdered(destKey, peerKey, bestKey):
			newCandidate(peerKey, peer.port)
		}
	}
	if canlength-PortCount == 0 {
		return types.SwitchPorts{0}
	}
	return candidates[canlength:]
}

func (t *virtualSnake) getVirtualSnakeTeardownNextHop(from *Peer, rx *types.Frame) types.SwitchPorts {
	t.ascendingMutex.Lock()
	if asc := t.ascending; asc != nil && asc.PublicKey == rx.DestinationKey {
		t.ascending = nil
		t.maintainNow.Dispatch()
	}
	t.ascendingMutex.Unlock()

	t.descendingMutex.Lock()
	if desc := t.descending; desc != nil && desc.PublicKey == rx.DestinationKey {
		t.descending = nil
		t.maintainNow.Dispatch()
	}
	t.descendingMutex.Unlock()

	t.tableMutex.Lock()
	defer t.tableMutex.Unlock()
	for k, v := range t.table {
		if k == rx.DestinationKey {
			delete(t.table, k)
			if from.port == v.SourcePort {
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
	var key types.PublicKey
	for k, v := range t.table {
		if v.DestinationPort == port || v.SourcePort == port {
			key = k
			break
		}
	}
	t.tableMutex.RUnlock()
	t.r.send <- types.Frame{
		Type:           types.TypeVirtualSnakeTeardown,
		DestinationKey: key,
	}
}

func (t *virtualSnake) clearRoutingEntriesForPublicKey(pk types.PublicKey) {
	t.r.send <- types.Frame{
		Type:           types.TypeVirtualSnakeTeardown,
		DestinationKey: pk,
	}
}

// handleBootstrap is called in response to an incoming bootstrap
// packet. It will update the descending information and send a setup
// message if needed.
func (t *virtualSnake) handleBootstrap(from *Peer, rx *types.Frame) {
	if rx.DestinationKey.EqualTo(t.r.public) {
		return
	}
	// Check if the packet has a valid signed timestamp.
	if !util.VerifySignedTimestamp(rx.DestinationKey, rx.Payload) {
		return
	}
	ts, err := util.SignedTimestamp(t.r.private)
	if err != nil {
		return
	}
	t.r.send <- types.Frame{
		Destination:    rx.Source,
		DestinationKey: rx.DestinationKey,
		Source:         t.r.Coords(),
		SourceKey:      t.r.PublicKey(),
		Type:           types.TypeVirtualSnakeBootstrapACK,
		Payload:        ts,
	}
}

func (t *virtualSnake) handleBootstrapACK(from *Peer, rx *types.Frame) {
	// Check if the packet has a valid signed timestamp.
	if !util.VerifySignedTimestamp(rx.SourceKey, rx.Payload) {
		return
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
	case t.ascending != nil && t.ascending.PublicKey.EqualTo(rx.SourceKey):
		// We've received another bootstrap ACK from our direct ascending node.
		// Just refresh the record and then send a new path setup message to
		// that node.
		t.ascending.Coords = rx.Source
		t.ascending.Port = from.port
		t.ascending.LastSeen = time.Now()
		ts, err := util.SignedTimestamp(t.r.private)
		if err != nil {
			return
		}
		send = &types.Frame{
			Destination:    t.ascending.Coords,
			DestinationKey: t.ascending.PublicKey,
			SourceKey:      t.r.public,
			Type:           types.TypeVirtualSnakeSetup,
			Payload:        ts,
		}
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
		//if t.ascending != nil && rx.SourceKey != t.ascending.PublicKey {
		//	t.clearRoutingEntriesForPublicKey(t.ascending.PublicKey)
		//}
		t.ascending = &virtualSnakeNeighbour{
			PublicKey: rx.SourceKey,
			Port:      from.port,
			LastSeen:  time.Now(),
			Coords:    rx.Source,
		}
		ts, err := util.SignedTimestamp(t.r.private)
		if err != nil {
			return
		}
		send = &types.Frame{
			Destination:    t.ascending.Coords,
			DestinationKey: t.ascending.PublicKey,
			SourceKey:      t.r.public,
			Type:           types.TypeVirtualSnakeSetup,
			Payload:        ts,
		}
	default:
		// The bootstrap ACK conditions weren't met. This might just be because
		// there's a node out there that hasn't converged to a closer node
		// yet, so we'll just ignore the acknowledgement.
	}
}

// handleSetup is called in response to an incoming path setup packet. Note
// that the setup packet isn't necessarily destined for us directly, but is
// instead called for every setup packet being transited through this node.
// It will update the routing table with the new path.
func (t *virtualSnake) handleSetup(from *Peer, rx *types.Frame, nextHops types.SwitchPorts) {
	// Check if the setup packet has a valid signed timestamp.
	if !util.VerifySignedTimestamp(rx.SourceKey, rx.Payload) {
		return
	}

	// Add a new routing table entry.
	// TODO: The routing table needs to be bounded by size, so that we don't
	// exhaust available system memory trying to maintain network paths. To
	// bound the routing table safely, we may want to make sure that we have
	// a reasonable spread of routes across keyspace so that we don't create
	// any obvious routing holes.
	entry := virtualSnakeEntry{
		LastSeen:   time.Now(),
		SourcePort: from.port,
	}
	if len(nextHops) > 0 {
		entry.DestinationPort = nextHops[0]
	}
	t.tableMutex.Lock()
	t.table[rx.SourceKey] = entry
	t.tableMutex.Unlock()

	// If we're at the destination of the setup then update our predecessor
	// with information from the bootstrap.
	if nextHops.EqualTo(types.SwitchPorts{0}) {
		t.descendingMutex.Lock()
		defer t.descendingMutex.Unlock()
		switch {
		case rx.SourceKey.EqualTo(t.r.public):
			// We received a bootstrap from ourselves. This shouldn't happen,
			// so either another node has forwarded it to us incorrectly, or
			// a routing loop has occurred somewhere. Don't act on the bootstrap
			// in that case.
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
			if t.descending != nil && rx.SourceKey != t.descending.PublicKey {
				t.clearRoutingEntriesForPublicKey(t.descending.PublicKey)
			}
			t.descending = &virtualSnakeNeighbour{
				PublicKey: rx.SourceKey,
				Port:      from.port,
				LastSeen:  time.Now(),
				Coords:    rx.Source,
			}
		default:
			// The bootstrap conditions weren't met. This might just be because
			// there's a node out there that hasn't converged to a closer node
			// yet, so we'll just ignore the bootstrap.
		}
	}
}
