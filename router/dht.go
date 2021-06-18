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
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
)

type dhtEntry interface {
	PublicKey() types.PublicKey
	Coordinates() types.SwitchPorts
	SeenRecently() bool
}

type dht struct {
	r           *Router
	finger      map[types.PublicKey]dhtEntry
	fingerMutex sync.RWMutex
	sorted      []dhtEntry
	sortedMutex sync.RWMutex
	requests    sync.Map
}

// setup sets up the DHT struct.
func newDHT(r *Router) *dht {
	d := &dht{
		r:      r,
		finger: make(map[types.PublicKey]dhtEntry),
	}
	return d
}

// table returns a set of known DHT neighbours to try querying.
func (d *dht) table() []dhtEntry {
	d.sortedMutex.RLock()
	defer d.sortedMutex.RUnlock()

	results := make([]dhtEntry, 0, len(d.sorted))
	for _, n := range d.sorted {
		if n.SeenRecently() {
			results = append(results, n)
		}
	}

	if len(results) > 8 {
		return results[:8]
	}
	return results
}

// getCloser returns a list of all nodes that we know about,
// either from the finger table or from the predecessor or
// successor which are closer to the given public key. It is
// possible for this list to be empty if we don't know of any
// closer nodes.
func (d *dht) getCloser(k types.PublicKey) []dhtEntry {
	results := append([]dhtEntry{}, d.sorted...)
	sort.SliceStable(results, func(i, j int) bool {
		return util.DHTOrdered(d.r.public, results[i].PublicKey(), results[j].PublicKey())
	})
	return results
}

// search starts an iterative DHT search. It will start
// the search by looking at nodes which we know about, and
// we will continue to ask nodes as we get closer to our
// destination. This function blocks for the duration of
// the search. The address returned will be a set of coords
// for greedy routing.
func (d *dht) search(ctx context.Context, pk, mask types.PublicKey, stopshort bool) (types.PublicKey, net.Addr, error) {
	// If the search is for our own key then don't do anything
	// as we already know what our own coordinates are.
	if d.r.public.EqualMaskTo(pk, mask) && !stopshort {
		return pk, &GreedyAddr{d.r.Coords()}, nil
	}

	// Each time we visit a node, we'll insert it into this
	// map. We specifically put our own key in here because
	// we shouldn't ever be directed back to ourselves as a
	// part of a search.
	var visited map[types.PublicKey]struct{}

	// The nexthop function processes the next step of the
	// search by contacting the next hop. This is recursive
	// and will continue until either there are no branches
	// to follow or we've reached our destination.
	var nexthop func(ctx context.Context, sc types.SwitchPorts, via, pk types.PublicKey) (types.PublicKey, types.SwitchPorts, error)
	nexthop = func(ctx context.Context, sc types.SwitchPorts, via, pk types.PublicKey) (types.PublicKey, types.SwitchPorts, error) {
		// Check that we haven't exceeded the supplied context.
		select {
		case <-ctx.Done():
			return via, nil, ctx.Err()
		default:
		}

		// Check that we haven't already visited this node before.
		// If we have then we won't contact it again.
		if _, ok := visited[via]; ok {
			return via, nil, fmt.Errorf("this node has already been visited")
		}

		// Mark the node as visited.
		visited[via] = struct{}{}

		// Send the request to the remote node. This will hopefully
		// return the node's own public key and a list of closer
		// nodes.
		pubkey, results, err := d.request(sc, pk)
		if err != nil {
			return via, nil, fmt.Errorf("d.request: %w", err)
		}

		// If the node's public key is equal to the public key that
		// we're looking for, then we've successfully completed the
		// search! Return the coordinates that we contacted.
		if pubkey.EqualMaskTo(pk, mask) {
			return pubkey, sc, nil
		}

		// Sort the resulting closer nodes by distance to the key
		// that we're searching for. This means we'll try nodes that
		// are closer in keyspace first, to hopefully shorten the
		// path that the search takes as far as possible. This is
		// also important because it means that, if the remote node
		// tried to send us deliberately out of our way, that we
		// will not try those distant nodes until we've at least
		// tried better ones first.
		sort.SliceStable(results, func(i, j int) bool {
			return util.DHTOrdered(pk, results[i].PublicKey, results[j].PublicKey)
		})

		// Process each of the results.
		for _, result := range results {
			if result.PublicKey.EqualMaskTo(pk, mask) && stopshort {
				return via, sc, nil
			}

			if k, c, err := nexthop(ctx, result.Coordinates.Copy(), result.PublicKey, pk); err == nil {
				// If there's no error then that means we've successfully
				// reached the destination node. Return the coordinates.
				// This will be passed back up the stack to the caller.
				return k, c, nil
			}
		}

		// If we've reached this point then none of the nodes that
		// were given to us returned satisfactory results. It may
		// be because they didn't know any closer nodes, or they
		// timed out/just didn't respond in a timely fashion.
		return via, nil, errors.New("search has reached dead end")
	}

	// Start the search by looking at our own predecessor,
	// successor and finger table.
	initial := d.table()
	sort.SliceStable(initial, func(i, j int) bool {
		return util.DHTOrdered(pk, initial[i].PublicKey(), initial[j].PublicKey())
	})
	for _, n := range initial {
		// Each time we start a search using one of our direct
		// neighbors, we'll reset the list of visited nodes. This
		// means that we don't accidentally kill searches based
		// on the results of a previous one. We insert our own
		// public key here so that if anyone refers us back to
		// ourselves, we will abort the search via that peer.
		visited = map[types.PublicKey]struct{}{
			d.r.public: {},
		}

		// Send the search via this peer. This is a recursive
		// function so will block until either the search fails
		// or completes. If the search fails, continue onto the
		// next peer and try that instead.
		key, found, err := nexthop(ctx, n.Coordinates(), n.PublicKey(), pk)
		if err != nil {
			continue
		}

		// We successfully found the destination, so return the
		// greedy coordinates as a net.Addr.
		return key, &GreedyAddr{found}, nil
	}

	return types.PublicKey{}, nil, errors.New("search failed via all known nodes")
}

// request sends a DHT query to a remote node on the network. To
// do this we need to know what the coordinates of the remote node
// are. The function will return the public key of the node queried
// and any closer DHT nodes that they know about, if possible.
func (d *dht) request(coords types.SwitchPorts, pk types.PublicKey) (types.PublicKey, []types.DHTNode, error) {
	// Create a new request context.
	dhtReq, requestID := d.newRequest()
	defer dhtReq.cancel()
	defer close(dhtReq.ch)
	defer d.requests.Delete(dhtReq.id)

	// Build the request query.
	req := types.DHTQueryRequest{}
	copy(req.PublicKey[:], pk[:])
	copy(req.RequestID[:], requestID[:])

	// Marshal it into binary so that we can send the request out
	// to the network.
	var buffer [MaxPayloadSize]byte
	n, err := req.MarshalBinary(buffer[:])
	if err != nil {
		return types.PublicKey{}, nil, fmt.Errorf("res.MarshalBinary: %w", err)
	}

	// Send the request frame to the switch. The switch will then
	// forward it onto the destination as appropriate.
	d.r.send <- types.Frame{
		Source:      d.r.Coords(),
		Destination: coords,
		Type:        types.TypeDHTRequest,
		Payload:     buffer[:n],
	}

	// Wait for a response that matches our search ID, or for the
	// timeout to kick in instead.
	select {
	case res := <-dhtReq.ch:
		return res.PublicKey, res.Results, nil
	case <-dhtReq.ctx.Done():
		return types.PublicKey{}, nil, fmt.Errorf("request timed out")
	}
}

// dhtRequestContext represents an ongoing DHT request.
type dhtRequestContext struct {
	id     [8]byte
	ctx    context.Context
	cancel context.CancelFunc
	ch     chan types.DHTQueryResponse
}

// newRequest creates a new dhtRequestContext and returns it, along
// with the randomised ID for the search.
func (d *dht) newRequest() (*dhtRequestContext, [8]byte) {
	var requestID [8]byte
	for {
		// Try generating a random search ID.
		n, err := rand.Read(requestID[:8])
		if n != 8 || err != nil {
			panic("failed to generate new request ID")
		}

		// If we've picked a search ID that's already in use then
		// keep trying until we hit one that is new.
		if _, ok := d.requests.Load(requestID); ok {
			continue
		}

		// Build a new timeout and response channel. If we get a
		// DHT response back, it will be sent into the matching
		// channel.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		ch := make(chan types.DHTQueryResponse, 1)
		req := &dhtRequestContext{requestID, ctx, cancel, ch}
		d.requests.Store(requestID, req)
		return req, requestID
	}
}

func (d *dht) insertNode(node dhtEntry) {
	id := node.PublicKey()

	// TODO: Check if the node is important to the functioning
	// of the DHT, e.g. by being in optimal ring positions to
	// reduce the path of the search.

	d.fingerMutex.Lock()
	if n, ok := d.finger[id]; ok {
		if _, ok = n.(*Peer); !ok {
			d.finger[id] = node
		}
	} else {
		d.finger[id] = node
	}
	d.fingerMutex.Unlock()

	d.rebuildSorted()
}

func (d *dht) deleteNode(id types.PublicKey) {
	d.fingerMutex.Lock()
	delete(d.finger, id)
	d.fingerMutex.Unlock()

	d.rebuildSorted()
}

func (d *dht) rebuildSorted() {
	d.sortedMutex.Lock()
	defer d.sortedMutex.Unlock()

	d.sorted = make([]dhtEntry, 0, len(d.finger))
	d.fingerMutex.RLock()
	for _, n := range d.finger {
		d.sorted = append(d.sorted, n)
	}
	d.fingerMutex.RUnlock()

	sort.SliceStable(d.sorted, func(i, j int) bool {
		return util.DHTOrdered(d.r.public, d.sorted[i].PublicKey(), d.sorted[j].PublicKey())
	})
}

// onDHTRequest is called when the router receives a DHT request.
func (d *dht) onDHTRequest(req *types.DHTQueryRequest, from types.SwitchPorts) {
	// Build a response.
	res := types.DHTQueryResponse{
		RequestID: req.RequestID,
	}
	copy(res.PublicKey[:], d.r.public[:])

	// Look up all nodes that we know about that are closer to
	// the public key being searched.
	for _, f := range d.getCloser(req.PublicKey) {
		node := types.DHTNode{
			PublicKey:   f.PublicKey(),
			Coordinates: f.Coordinates(),
		}
		res.Results = append(res.Results, node)
	}

	// Marshal the response into binary format so we can send it
	// back.
	var buffer [MaxPayloadSize]byte
	n, err := res.MarshalBinary(buffer[:], d.r.private[:])
	if err != nil {
		fmt.Println("Failed to sign DHT response:", err)
		return
	}

	// Send the DHT response back to the requestor.
	d.r.send <- types.Frame{
		Source:      d.r.Coords(),
		Destination: from,
		Type:        types.TypeDHTResponse,
		Payload:     buffer[:n],
	}
}

// onDHTResponse is called when the router receives a DHT response.
func (d *dht) onDHTResponse(res *types.DHTQueryResponse, from types.SwitchPorts) {
	/*
		ctx, cancel := context.WithCancel(d.r.context)
		d.insertNode(&dhtNode{
			ctx:      ctx,
			cancel:   cancel,
			public:   res.PublicKey,
			coords:   from.Copy(),
			lastseen: time.Now(),
		})
	*/

	req, ok := d.requests.Load(res.RequestID)
	if ok {
		dhtReq, ok := req.(*dhtRequestContext)
		if ok && dhtReq.id == res.RequestID {
			dhtReq.ch <- *res
			d.requests.Delete(res.RequestID)
		}
	}
}
