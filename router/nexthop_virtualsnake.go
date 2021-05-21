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
	"fmt"
	"math"
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

type virtualSnakeTable map[types.PublicKey]virtualSnakeEntry

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
		if ascending != nil && time.Since(ascending.LastSeen) > virtualSnakeNeighExpiryPeriod {
			ascending = nil
		}
		v.ascendingMutex.RUnlock()
		v.descendingMutex.RLock()
		descending := v.descending
		if descending != nil && time.Since(descending.LastSeen) > virtualSnakeNeighExpiryPeriod {
			descending = nil
		}
		v.descendingMutex.RUnlock()

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
				v.r.log.Println("*** SENDING BOOTSTRAP FROM", v.r.Coords())
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

// getVirtualSnakeNextHop will return the most relevant port
// for a given destination public key.
func (t *virtualSnake) getVirtualSnakeNextHop(from *Peer, destKey types.PublicKey, bootstrap bool) types.SwitchPorts {
	if !bootstrap && t.r.public.EqualTo(destKey) {
		return types.SwitchPorts{0}
	}

	ancestors, parentPort := t.r.tree.Ancestors()
	rootKey := t.r.RootPublicKey()
	bestKey, bestPort := t.r.public, types.SwitchPortID(0) // rootKey, rootPort
	newCandidate := func(key types.PublicKey, port types.SwitchPortID) {
		bestKey, bestPort = key, port
	}
	newCheckedCandidate := func(candidate types.PublicKey, port types.SwitchPortID) bool {
		switch {
		case !bootstrap && candidate.EqualTo(destKey) && !bestKey.EqualTo(destKey):
			newCandidate(candidate, port)
			return true
		case util.DHTOrdered(destKey, candidate, bestKey):
			newCandidate(candidate, port)
			return true
		}
		return false
	}

	// Check if we can use the path to the root as a starting point
	switch {
	case bootstrap && bestKey.EqualTo(destKey):
		// Bootstraps always start working towards the root so that
		// they go somewhere rather than getting stuck
		newCandidate(rootKey, parentPort)
	case util.DHTOrdered(bestKey, destKey, rootKey):
		// The destination key is higher than our own key, so
		// start using the path to the root as the first candidate
		newCandidate(rootKey, parentPort)
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

	// Check our direct ancestors
	// bestKey <= destKey < rootKey
	for _, ancestor := range ancestors {
		newCheckedCandidate(ancestor, parentPort)
	}

	// Check our direct peers
	if bootstrap {
		for _, peer := range t.r.activePorts() {
			peerKey := peer.PublicKey()
			switch {
			case newCheckedCandidate(peer.public, peer.port):
			// This peer is closer to the candidate paths so far
			case bestKey.EqualTo(peerKey):
				// We've seen this key already, either as one of our ancestors
				// or as an ancestor of one of our peers, but it turns out we
				// are directly peered with that node, so use the more direct
				// path instead
				newCandidate(peerKey, peer.port)
			}
		}
	}

	// Check our DHT entries
	t.tableMutex.RLock()
	for dhtKey, entry := range t.table {
		if entry.Valid() {
			newCheckedCandidate(dhtKey, entry.SourcePort)
		}
	}
	t.tableMutex.RUnlock()

	return types.SwitchPorts{bestPort}
}

func (t *virtualSnake) getVirtualSnakeTeardownNextHop(from *Peer, rx *types.Frame) types.SwitchPorts {
	if len(rx.Payload) < 1 {
		return types.SwitchPorts{}
	}
	ascending := rx.Payload[0] == 1
	changed := false
	if ascending {
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
		if k == rx.DestinationKey {
			delete(t.table, k)
			if ascending {
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
	var asc []types.PublicKey
	var desc []types.PublicKey
	for k, v := range t.table {
		if v.DestinationPort == port {
			asc = append(asc, k)
		}
		if v.SourcePort == port {
			desc = append(desc, k)
		}
	}
	t.tableMutex.RUnlock()
	for _, k := range asc {
		t.r.send <- types.Frame{
			Type:           types.TypeVirtualSnakeTeardown,
			SourceKey:      t.r.public,
			DestinationKey: k,
			Payload:        []byte{1},
		}
	}
	for _, k := range desc {
		t.r.send <- types.Frame{
			Type:           types.TypeVirtualSnakeTeardown,
			SourceKey:      t.r.public,
			DestinationKey: k,
			Payload:        []byte{0},
		}
	}
}

func (t *virtualSnake) clearRoutingEntriesForPublicKey(pk types.PublicKey, ascending bool) {
	payload := []byte{0}
	if ascending {
		payload[0] = 1
	}
	t.r.send <- types.Frame{
		Type:           types.TypeVirtualSnakeTeardown,
		SourceKey:      t.r.public,
		DestinationKey: pk,
		Payload:        payload,
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
	t.r.log.Println("*** SENDING BOOTSTRAP ACK FROM", t.r.Coords())
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
		if t.ascending != nil && rx.SourceKey != t.ascending.PublicKey {
			t.clearRoutingEntriesForPublicKey(t.ascending.PublicKey, true)
		}
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
			Destination:    rx.Source,
			DestinationKey: rx.SourceKey,
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
func (t *virtualSnake) handleSetup(from *Peer, rx *types.Frame, nextHops types.SwitchPorts) error {
	// Check if the setup packet has a valid signed timestamp.
	if !util.VerifySignedTimestamp(rx.SourceKey, rx.Payload) {
		return fmt.Errorf("invalid signature")
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

	// Did the setup hit a dead end on the way to the ascending node?
	if nextHops.EqualTo(types.SwitchPorts{0}) && !rx.DestinationKey.EqualTo(t.r.public) {
		t.clearRoutingEntriesForPublicKey(rx.SourceKey, false)
		return fmt.Errorf("setup hit dead end")
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
				t.clearRoutingEntriesForPublicKey(t.descending.PublicKey, false)
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

	return nil
}
