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
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/router/events"
	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

type Router struct {
	phony.Inbox
	log           types.Logger
	context       context.Context
	cancel        context.CancelFunc
	public        types.PublicKey
	private       types.PrivateKey
	active        sync.Map
	local         *peer
	state         *state
	secure        bool
	_hopLimiting  *atomic.Bool
	_readDeadline *atomic.Time
	_subscribers  map[chan<- events.Event]*phony.Inbox
}

func NewRouter(logger types.Logger, sk ed25519.PrivateKey, opts ...RouterOption) *Router {
	if logger == nil {
		logger = log.New(ioutil.Discard, "", 0)
	}
	blackhole := false
	for _, opt := range opts {
		switch v := opt.(type) {
		case RouterOptionBlackhole:
			blackhole = bool(v)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, insecure := os.LookupEnv("PINECONE_DISABLE_SIGNATURES")
	r := &Router{
		log:           logger,
		context:       ctx,
		cancel:        cancel,
		secure:        !insecure,
		_hopLimiting:  atomic.NewBool(false),
		_readDeadline: atomic.NewTime(time.Now().Add(time.Hour * 24 * 365 * 100)), // ~100 years
		_subscribers:  make(map[chan<- events.Event]*phony.Inbox),
	}
	// Populate the node keys from the supplied private key.
	copy(r.private[:], sk)
	r.public = r.private.Public()
	// Create a state actor.
	r.state = &state{
		r:             r,
		_table:        make(virtualSnakeTable),
		_peers:        make([]*peer, portCount),
		_filterPacket: nil,
	}
	// Create a new local peer and wire it into port 0.
	r.local = r.newLocalPeer(blackhole)
	r.state._peers[0] = r.local
	// Start the state actor.
	r.state.Act(nil, r.state._start)
	r.log.Println("Router identity:", r.public.String())

	return r
}

func (r *Router) InjectPacketFilter(fn FilterFn) {
	phony.Block(r.state, func() {
		r.state._filterPacket = fn
	})
}

func (r *Router) EnableWakeupBroadcasts() {
	r.state.Act(r.state, func() {
		r.state._sendBroadcastIn(0)
	})
}

func (r *Router) DisableWakeupBroadcasts() {
	r.state.Act(r.state, func() {
		if !r.state._broadcastTimer.Stop() {
			<-r.state._broadcastTimer.C
		}
	})
}

// _publish notifies each subscriber of a new event.
func (r *Router) _publish(event events.Event) {
	for ch, inbox := range r._subscribers {
		// Create a copy of the pointer before passing into the lambda
		chCopy := ch
		inbox.Act(nil, func() {
			chCopy <- event
		})
	}
}

// IsConnected returns true if the node is connected within the
// given zone, or false otherwise.
func (r *Router) IsConnected(key types.PublicKey, zone string) bool {
	v, ok := r.active.Load(hex.EncodeToString(key[:]) + zone)
	if !ok {
		return false
	}
	count := v.(*atomic.Uint64)
	return count.Load() > 0
}

// Close will stop the Pinecone node. Once this has been called, the node cannot
// be restarted or reused.
func (r *Router) Close() error {
	phony.Block(r, func() {
		if r.cancel != nil {
			r.cancel()
		}
	})
	return nil
}

// PrivateKey returns the private key of the node.
func (r *Router) PrivateKey() types.PrivateKey {
	return r.private
}

// PublicKey returns the public key of the node.
func (r *Router) PublicKey() types.PublicKey {
	return r.public
}

// Addr returns the local address of the node in the form of a `types.PublicKey`.
func (r *Router) Addr() net.Addr {
	return r.PublicKey()
}

// Connect takes a connection and attaches it to the switch as a peering. This
// function takes one or more ConnectionOptions to configure the peer. If no
// ConnectionPublicKey is specified, the connection will autonegotiate with the
// remote peer to exchange public keys and version/capability information.
func (r *Router) Connect(conn net.Conn, options ...ConnectionOption) (types.SwitchPortID, error) {
	var public types.PublicKey
	var uri ConnectionURI
	var zone ConnectionZone
	var peertype ConnectionPeerType
	keepalives := true
	for _, option := range options {
		switch v := option.(type) {
		case ConnectionPublicKey:
			public = types.PublicKey(v)
		case ConnectionURI:
			uri = v
		case ConnectionZone:
			zone = v
		case ConnectionPeerType:
			peertype = v
		case ConnectionKeepalives:
			keepalives = bool(v)
		}
	}

	var empty types.PublicKey
	if public == empty {
		handshake := []byte{
			ourVersion,
			0, // unused
			0, // unused
			0, // unused
			0, // capabilities
			0, // capabilities
			0, // capabilities
			0, // capabilities
		}
		binary.BigEndian.PutUint32(handshake[4:8], ourCapabilities)
		handshake = append(handshake, r.public[:ed25519.PublicKeySize]...)
		handshake = append(handshake, ed25519.Sign(r.private[:], handshake)...)
		if err := conn.SetDeadline(time.Now().Add(time.Second * 10)); err != nil {
			return 0, fmt.Errorf("conn.SetDeadline: %w", err)
		}
		if _, err := conn.Write(handshake); err != nil {
			conn.Close()
			return 0, fmt.Errorf("conn.Write: %w", err)
		}
		if _, err := io.ReadFull(conn, handshake); err != nil {
			conn.Close()
			return 0, fmt.Errorf("io.ReadFull: %w", err)
		}
		if err := conn.SetDeadline(time.Time{}); err != nil {
			return 0, fmt.Errorf("conn.SetDeadline: %w", err)
		}
		if theirVersion := handshake[0]; theirVersion != ourVersion {
			conn.Close()
			return 0, fmt.Errorf("mismatched node version")
		}
		if theirCapabilities := binary.BigEndian.Uint32(handshake[4:8]); theirCapabilities != ourCapabilities {
			conn.Close()
			return 0, fmt.Errorf("mismatched node capabilities")
		}
		var signature types.Signature
		offset := 8
		offset += copy(public[:], handshake[offset:offset+ed25519.PublicKeySize])
		copy(signature[:], handshake[offset:offset+ed25519.SignatureSize])
		if !ed25519.Verify(public[:], handshake[:offset], signature[:]) {
			conn.Close()
			return 0, fmt.Errorf("peer sent invalid signature")
		}
	}

	port := types.SwitchPortID(0)
	var err error
	phony.Block(r.state, func() {
		port, err = r.state._addPeer(conn, public, uri, zone, peertype, keepalives)
	})
	if err != nil {
		return types.SwitchPortID(0), fmt.Errorf("_addPeer: %w", err)
	}
	return port, nil
}

// Disconnect will disconnect whatever is connected to the
// given port number on the Pinecone node. The peering will
// no longer be used and the underlying connection will be
// closed.
func (r *Router) Disconnect(i types.SwitchPortID, err error) {
	if i == 0 {
		return
	}
	phony.Block(r.state, func() {
		if p := r.state._peers[i]; p != nil && p.started.Load() && p != r.local {
			p.stop(err)
		}
	})
}

// PeerCount returns the number of nodes that are directly
// connected to this Pinecone node.
func (r *Router) PeerCount(peertype int) (count int) {
	phony.Block(r.state, func() {
		seen := map[types.PublicKey]struct{}{}
		for _, p := range r.state._peers {
			if p == nil || p.port == 0 || !p.started.Load() {
				continue
			}
			if int(p.peertype) == peertype || peertype < 0 {
				if _, ok := seen[p.public]; !ok {
					count++
				}
				seen[p.public] = struct{}{}
			}
		}
	})
	return
}

// TotalPeerCount returns the total number of nodes that are directly connected
// to this Pinecone node.
func (r *Router) TotalPeerCount() (count int) {
	// PeerCount treats values < 0 specially, returning the count of all connected
	// peers, regardless of type.
	return r.PeerCount(-1)
}
