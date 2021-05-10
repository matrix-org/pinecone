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
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/types"
	"github.com/matrix-org/pinecone/util"
)

// PortCount contains the number of ports supported by this Pinecone
// node. This is, in practice, the limit of concurrent peering
// connections that are supported at one time.
const PortCount = 64

// ProtoBufferSize is the number of protocol packets that a node will
// buffer on a slow port.
const ProtoBufferSize = 16

// TrafficBufferSize is the number of traffic packets that a node will
// buffer on a slow port.
const TrafficBufferSize = 256

// Simulator is not used by normal Pinecone nodes and specifies the
// functions that must be satisfied if running under pineconesim.
type Simulator interface {
	ReportDistance(a, b string, l int64)
	LookupCoords(string) (types.SwitchPorts, error)
	LookupNodeID(types.SwitchPorts) (string, error)
	LookupPublicKey(types.PublicKey) (string, error)
	ReportNewLink(net.Conn, types.PublicKey, types.PublicKey)
	ReportDeadLink(types.PublicKey, types.PublicKey)
}

// Implements net.PacketConn. A Router is an instance of a Pinecone
// node and should only be instantiated using the NewRouter method.
type Router struct {
	log        *log.Logger        //
	context    context.Context    // switch context
	cancel     context.CancelFunc // switch context shutdown signal
	callbacks  *callbacks         // notify when something happens
	ports      [PortCount]*Peer   // all switch ports
	simulator  Simulator          // is the node running in the sim?
	id         string             // friendly identifier (for sim)
	private    types.PrivateKey   // our keypair
	public     types.PublicKey    // our keypair
	tree       *spanningTree      // Yggdrasil-like spanning tree
	snake      *virtualSnake      // SNEK routing protocol
	dht        *dht               // Chord-like DHT tables
	pathfinder *pathfinder        // source routing pathfinder
	active     sync.Map           // node public keys that we have active peerings with
	send       chan types.Frame   // local node -> network
	recv       chan types.Frame   // local node <- network
}

// NewRouter instantiates a new Pinecone Router instance. The logger
// is where all log output will be sent for this node. The ID is a
// friendly string that identifies the node, but is not used at the
// protocol level and is not visible outside externally. The private
// and public keys are the primary identity of the node and therefore
// must be unique for each node. These keys are also used to sign
// protocol messages.
func NewRouter(log *log.Logger, id string, private ed25519.PrivateKey, public ed25519.PublicKey, simulator Simulator) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	sw := &Router{
		log:       log,
		context:   ctx,
		cancel:    cancel,
		id:        id,
		simulator: simulator,
		send:      make(chan types.Frame, 32),
		recv:      make(chan types.Frame, 32),
	}
	sw.callbacks = &callbacks{r: sw}
	copy(sw.private[:], private)
	copy(sw.public[:], public)
	sw.log.Println("Switch public key:", hex.EncodeToString(public))

	// Prepare the switch ports.
	for i := range sw.ports {
		sw.ports[i] = &Peer{
			r:    sw,
			port: types.SwitchPortID(i),
		}
	}

	// A Pinecone node implements a few different things: a spanning
	// tree, a Chord-like DHT and a source routing pathfinder. Each
	// of these.
	sw.tree = newSpanningTree(sw, func(parent types.SwitchPortID, coords types.SwitchPorts) {
		sw.log.Println("New coordinates:", coords)
	})
	sw.dht = newDHT(sw)
	sw.pathfinder = newPathfinder(sw)
	sw.snake = newVirtualSnake(sw)

	// Previously the router was in a separate package so we still
	// wire up local traffic to a switch port. TODO: We should really
	// change this so that peer.go handles local traffic.
	pipelocal, pipeswitch := net.Pipe()
	if id, err := sw.Connect(pipeswitch, sw.public, "router", -1); err != nil {
		panic(err)
	} else if id != 0 {
		panic("router must be port 0")
	}
	go sw.reader(pipelocal)
	go sw.writer(pipelocal)
	go sw.startManhole()

	return sw
}

// Close shuts down the node and stops the peering connections.
// The node should not be used after this has been called.
func (r *Router) Close() error {
	r.cancel()
	for _, port := range r.ports {
		if port.started.Load() {
			_ = port.stop()
		}
	}
	return nil
}

// PrivateKey returns the ed25519 private key in use by this node.
func (r *Router) PrivateKey() types.PrivateKey {
	return r.private
}

// PublicKey returns the ed25519 public key in use by this node.
func (r *Router) PublicKey() types.PublicKey {
	return r.public
}

// Coords returns the current spanning tree coordinates of
// this node. The coordinates are effectively a source routing
// path from the root down to the node.
func (r *Router) Coords() types.SwitchPorts {
	return r.tree.Coords()
}

// RootPublicKey returns the public key of the node that this
// node believes is the root of the network.
func (r *Router) RootPublicKey() types.PublicKey {
	return r.tree.Root().RootPublicKey
}

// IsRoot returns true if this node believes it is the root of
// the network. This will likely return true if the node is
// isolated (e.g. has no peers).
func (r *Router) IsRoot() bool {
	return r.tree.Root().RootPublicKey.EqualTo(r.public)
}

// Addr returns a net.Addr instance that addresses this node
// using greedy routing.
func (r *Router) Addr() net.Addr {
	return r.PublicKey()
}

// Pathfind takes a GreedyAddr (for greedy routing) or a types.PublicKey
// (for snake routing) and performs an active pathfind through the network.
// A SourceAddr will be returned which can be used to send packets using
// source routing instead. Note that this generates protocol traffic, so
// don't call it again unless you are sure the path has changed and the
// remote host is no longer answering. If the pathfind fails then an error
// will be returned instead.
func (r *Router) Pathfind(ctx context.Context, addr net.Addr) (net.Addr, error) {
	return r.pathfinder.pathfind(ctx, addr)
}

// DHTSearch initiates a DHT search for the given public key.
// The stopshort flag, if set to true, will return the closest
// node to the public key found in the search. If set to false,
// the search will fail and return an error if the specific node
// is not found.
func (r *Router) DHTSearch(ctx context.Context, pk ed25519.PublicKey, stopshort bool) (types.PublicKey, net.Addr, error) {
	var public types.PublicKey
	copy(public[:], pk)
	return r.dht.search(ctx, public, stopshort)
}

// DHTPredecessor returns the public key of the previous node in
// the DHT snake.
func (r *Router) Predecessor() *types.PublicKey {
	r.snake.descendingMutex.RLock()
	pr := r.snake.descending
	r.snake.descendingMutex.RUnlock()
	if pr == nil || time.Since(pr.LastSeen) >= virtualSnakeNeighExpiryPeriod {
		return nil
	}
	pk := pr.PublicKey
	return &pk
}

// DHTSuccessor returns the public key of the next node in the
// DHT snake.
func (r *Router) Successor() *types.PublicKey {
	r.snake.ascendingMutex.RLock()
	su := r.snake.ascending
	r.snake.ascendingMutex.RUnlock()
	if su == nil || time.Since(su.LastSeen) >= virtualSnakeNeighExpiryPeriod {
		return nil
	}
	pk := su.PublicKey
	return &pk
}

// KnownNodes returns a list of all nodes that are known about
// directly. This includes all peers and all ancestor nodes
// between this node and the root node.
func (r *Router) KnownNodes() []types.PublicKey {
	known := map[types.PublicKey]struct{}{}
	for _, p := range r.activePorts() {
		p.mutex.RLock()
		known[p.public] = struct{}{}
		p.mutex.RUnlock()
	}
	r.snake.descendingMutex.RLock()
	if p := r.snake.descending; p != nil {
		known[p.PublicKey] = struct{}{}
	}
	r.snake.descendingMutex.RUnlock()
	r.snake.ascendingMutex.RLock()
	if s := r.snake.ascending; s != nil {
		known[s.PublicKey] = struct{}{}
	}
	r.snake.ascendingMutex.RUnlock()
	r.snake.tableMutex.RLock()
	for k := range r.snake.table {
		known[k] = struct{}{}
	}
	r.snake.tableMutex.RUnlock()
	list := make([]types.PublicKey, 0, len(known))
	for n := range known {
		list = append(list, n)
	}
	return list
}

func (r *Router) activePorts() peers {
	peers := make(peers, 0, PortCount)
	for _, p := range r.ports {
		switch {
		case p.port == 0: // ignore the router
			continue
		case !p.started.Load() || !p.alive.Load(): // ignore stopped/non-negotiated ports
			continue
		default:
			peers = append(peers, p)
		}
	}
	sort.Sort(peers)
	return peers
}

// AuthenticatedConnect initiates a peer connection using
// the given net.Conn connection. The public keys of the
// nodes are exchanged using a handshake. The connection
// will fail if this handshake fails. The port number that
// the node was connected to will be returned in the event
// of a successful connection.
func (r *Router) AuthenticatedConnect(conn net.Conn, zone string, peertype int) (types.SwitchPortID, error) {
	select {
	case <-time.After(time.Second * 5):
		return 0, fmt.Errorf("handshake timed out")
	default:
		handshake := []byte{
			ourVersion,
			ourCapabilities,
			0, // unused
			0, // unused
		}
		handshake = append(handshake, r.public[:ed25519.PublicKeySize]...)
		handshake = append(handshake, ed25519.Sign(r.private[:], handshake)...)
		_ = conn.SetDeadline(time.Now().Add(time.Second * 5))
		if _, err := conn.Write(handshake); err != nil {
			conn.Close()
			return 0, err
		}
		if _, err := io.ReadFull(conn, handshake); err != nil {
			conn.Close()
			return 0, fmt.Errorf("io.ReadFull: %w", err)
		}
		_ = conn.SetDeadline(time.Time{})
		if theirVersion := handshake[0]; theirVersion != ourVersion {
			conn.Close()
			return 0, fmt.Errorf("mismatched node version")
		}
		if theirCapabilities := handshake[1]; theirCapabilities&ourCapabilities != ourCapabilities {
			conn.Close()
			return 0, fmt.Errorf("mismatched node capabilities")
		}
		var public types.PublicKey
		var signature types.Signature
		offset := 4
		offset += copy(public[:], handshake[offset:offset+ed25519.PublicKeySize])
		copy(signature[:], handshake[offset:offset+ed25519.SignatureSize])
		if !ed25519.Verify(public[:], handshake[:offset], signature[:]) {
			conn.Close()
			return 0, fmt.Errorf("peer sent invalid signature")
		}
		port, err := r.Connect(conn, public, zone, peertype)
		if err != nil {
			conn.Close()
			return 0, err
		}
		return port, nil
	}
}

// Connect initiates a peer connection using the given
// net.Conn connection, in the event that the public key of
// the node is already known (e.g. in the simulator). The
// port number that the node was connected to will be
// returned in the event of a successful connection.
func (r *Router) Connect(conn net.Conn, public types.PublicKey, zone string, peertype int) (types.SwitchPortID, error) {
	if p, ok := r.active.Load(hex.EncodeToString(public[:]) + zone); ok {
		if err := r.Disconnect(p.(types.SwitchPortID), nil); err != nil {
			return 0, fmt.Errorf("already connected to this node via zone %q", zone)
		}
	}
	for i := types.SwitchPortID(0); i < PortCount; i++ {
		if i != 0 && bytes.Equal(r.public[:], public[:]) {
			return 0, fmt.Errorf("loopback connection prohibited")
		}
		if r.ports[i].started.Load() {
			continue
		}
		r.ports[i].mutex.Lock()
		r.ports[i].context, r.ports[i].cancel = context.WithCancel(r.context)
		r.ports[i].zone = zone
		r.ports[i].peertype = peertype
		r.ports[i].conn = util.NewBufferedRWC(conn)
		r.ports[i].public = public
		r.ports[i].protoOut = make(chan *types.Frame, ProtoBufferSize)
		r.ports[i].trafficOut = make(chan *types.Frame, TrafficBufferSize)
		r.ports[i].advertise = util.NewDispatch()
		r.ports[i].statistics.reset()
		r.ports[i].mutex.Unlock()
		if err := r.ports[i].start(); err != nil {
			return 0, fmt.Errorf("port.start: %w", err)
		}
		if i != 0 {
			r.dht.insertNode(r.ports[i])
		}
		if r.simulator != nil {
			r.simulator.ReportNewLink(conn, r.public, public)
		}
		r.log.Printf("Connected port %d to %s (zone %q)\n", i, conn.RemoteAddr(), zone)
		go r.callbacks.onConnected(i, public, peertype)
		r.active.Store(hex.EncodeToString(public[:])+zone, i)
		return i, nil
	}
	return 0, fmt.Errorf("no free switch ports")
}

// Disconnect will disconnect whatever is connected to the
// given port number on the Pinecone node. The peering will
// no longer be used and the underlying connection will be
// closed.
func (r *Router) Disconnect(i types.SwitchPortID, err error) error {
	if i == 0 {
		return fmt.Errorf("cannot disconnect port %d", i)
	}
	r.active.Delete(hex.EncodeToString(r.ports[i].public[:]) + r.ports[i].zone)
	r.ports[i].mutex.Lock()
	_ = r.ports[i].stop()
	r.ports[i].peertype = 0
	r.ports[i].zone = ""
	r.ports[i].public = types.PublicKey{}
	if len(r.ports[i].protoOut) > 0 {
		for range r.ports[i].protoOut {
		}
	}
	if len(r.ports[i].trafficOut) > 0 {
		for range r.ports[i].trafficOut {
		}
	}
	r.ports[i].protoOut = nil
	r.ports[i].trafficOut = nil
	r.ports[i].mutex.Unlock()
	if r.ports[i].port != 0 {
		r.dht.deleteNode(r.ports[i].public)
	}
	if r.simulator != nil {
		r.simulator.ReportDeadLink(r.public, r.ports[i].public)
	}
	r.log.Printf("Disconnected port %d: %s\n", i, err)
	go r.callbacks.onDisconnected(i, r.ports[i].public, r.ports[i].peertype, err)
	r.snake.portWasDisconnected(i)
	r.tree.portWasDisconnected(i)
	return nil
}

// PeerCount returns the number of nodes that are directly
// connected to this Pinecone node. This will de-duplicate
// peerings to the same node in different zones.
func (r *Router) PeerCount(peertype int) int {
	count := 0
	ports := r.activePorts()
	if peertype < 0 {
		return len(ports)
	}
	for _, p := range ports {
		p.mutex.RLock()
		if p.peertype == peertype {
			count++
		}
		p.mutex.RUnlock()
	}
	return count
}

// IsConnected returns true if the node is connected within the
// given zone, or false otherwise.
func (r *Router) IsConnected(key types.PublicKey, zone string) bool {
	_, ok := r.active.Load(hex.EncodeToString(key[:]) + zone)
	return ok
}

// PeerInfo is a gomobile-friendly type that represents a peer
// connection.
type PeerInfo struct {
	Port      int
	PublicKey string
	PeerType  int
	Zone      string
}

// Peers returns a list of PeerInfos that show all of the currently
// connected peers.
func (r *Router) Peers() []PeerInfo {
	peers := make([]PeerInfo, 0, PortCount)
	for _, p := range r.activePorts() {
		p.mutex.RLock()
		peers = append(peers, PeerInfo{
			Port:      int(p.port),
			PeerType:  p.peertype,
			PublicKey: p.public.String(),
			Zone:      p.zone,
		})
		p.mutex.RUnlock()
	}
	return peers
}
