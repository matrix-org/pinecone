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
	"log"
	"math"
	"net"
	"sync"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

const PortCount = math.MaxUint8
const TrafficBuffer = math.MaxUint8

type Router struct {
	log        *log.Logger
	id         string
	debug      atomic.Bool
	simulator  Simulator
	context    context.Context
	cancel     context.CancelFunc
	public     types.PublicKey
	private    types.PrivateKey
	keepalives bool
	active     sync.Map
	pings      sync.Map // types.PublicKey -> chan struct{}
	local      *peer
	state      *state
}

func NewRouter(log *log.Logger, sk ed25519.PrivateKey, id string, sim Simulator) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Router{
		log:        log,
		id:         id,
		simulator:  sim,
		context:    ctx,
		cancel:     cancel,
		keepalives: sim == nil,
	}
	copy(r.private[:], sk)
	r.public = r.private.Public()
	r.state = &state{
		r:      r,
		_table: make(virtualSnakeTable),
		_peers: make([]*peer, PortCount),
	}
	r.local = r.localPeer()
	r.state._peers[0] = r.local
	r.state.Act(nil, r.state._start)
	r.log.Println("Router identity:", r.public.String())
	return r
}

func (r *Router) ToggleDebug() {
	if !r.debug.Toggle() {
		r.log.Println("Enabled debug logging")
	} else {
		r.log.Println("Disabled debug logging")
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

func (r *Router) Close() error {
	phony.Block(nil, r.cancel)
	return nil
}

func (r *Router) PrivateKey() types.PrivateKey {
	return r.private
}

func (r *Router) PublicKey() types.PublicKey {
	return r.public
}

func (r *Router) Addr() net.Addr {
	return r.PublicKey()
}

func (r *Router) Connect(conn net.Conn, public types.PublicKey, zone string, peertype int) (types.SwitchPortID, error) {
	var new *peer
	phony.Block(r.state, func() {
		for i, p := range r.state._peers {
			if i == 0 || p != nil {
				// Port 0 is reserved for the local router.
				// Already allocated ports should be ignored.
				continue
			}
			ctx, cancel := context.WithCancel(r.context)
			new = &peer{
				router:   r,
				port:     types.SwitchPortID(i),
				conn:     conn,
				public:   public,
				zone:     zone,
				peertype: peertype,
				context:  ctx,
				cancel:   cancel,
				proto:    newFIFOQueue(),
				traffic:  newLIFOQueue(TrafficBuffer),
			}
			new.started.Store(true)
			r.state._peers[i] = new
			r.log.Println("Connected to peer", new.public.String(), "on port", new.port)
			v, _ := r.active.LoadOrStore(hex.EncodeToString(new.public[:])+zone, atomic.NewUint64(0))
			v.(*atomic.Uint64).Inc()
			r.state.Act(&new.writer, func() {
				if !r.state._waiting {
					r.state.sendTreeAnnouncementToPeer(r.state._rootAnnouncement(), new)
				}
			})
			new.reader.Act(nil, new._read)
			new.writer.Act(nil, new._write)
			return
		}
	})
	if new == nil {
		return 0, fmt.Errorf("no free switch ports")
	}
	return new.port, nil
}

func (r *Router) AuthenticatedConnect(conn net.Conn, zone string, peertype int) (types.SwitchPortID, error) {
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
	if err := conn.SetDeadline(time.Now().Add(PeerKeepaliveInterval)); err != nil {
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
	if theirCapabilities := binary.BigEndian.Uint32(handshake[4:8]); theirCapabilities&ourCapabilities != ourCapabilities {
		conn.Close()
		return 0, fmt.Errorf("mismatched node capabilities")
	}
	var public types.PublicKey
	var signature types.Signature
	offset := 8
	offset += copy(public[:], handshake[offset:offset+ed25519.PublicKeySize])
	copy(signature[:], handshake[offset:offset+ed25519.SignatureSize])
	if !ed25519.Verify(public[:], handshake[:offset], signature[:]) {
		conn.Close()
		return 0, fmt.Errorf("peer sent invalid signature")
	}
	port, err := r.Connect(conn, public, zone, peertype)
	if err != nil {
		return 0, fmt.Errorf("r.Connect failed: %w (close: %s)", err, conn.Close())
	}
	return port, err
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
		if p := r.state._peers[i]; p != nil && p.started.Load() {
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
			if p == nil {
				continue
			}
			if p.peertype == peertype || peertype < 0 {
				if _, ok := seen[p.public]; !ok {
					count++
				}
				seen[p.public] = struct{}{}
			}
		}
	})
	return
}

func (r *Router) SNEKPing(ctx context.Context, dst types.PublicKey) (time.Duration, error) {
	if dst == r.public {
		return 0, nil
	}
	phony.Block(r.state, func() {
		frame := getFrame()
		frame.Type = types.TypeSNEKPing
		frame.DestinationKey = dst
		frame.SourceKey = r.public
		_ = r.state._forward(r.local, frame)
	})
	start := time.Now()
	v, existing := r.pings.LoadOrStore(dst, make(chan struct{}))
	if existing {
		return 0, fmt.Errorf("a ping to this node is already in progress")
	}
	defer r.pings.Delete(dst)
	ch := v.(chan struct{})
	select {
	case <-ctx.Done():
		return 0, fmt.Errorf("ping timed out")
	case <-ch:
		return time.Since(start), nil
	}
}

func (r *Router) TreePing(ctx context.Context, dst types.SwitchPorts) (time.Duration, error) {
	if dst.EqualTo(r.state.coords()) {
		return 0, nil
	}
	phony.Block(r.state, func() {
		frame := getFrame()
		frame.Type = types.TypeTreePing
		frame.Destination = dst
		frame.Source = r.state._coords()
		_ = r.state._forward(r.local, frame)
	})
	start := time.Now()
	v, existing := r.pings.LoadOrStore(dst.String(), make(chan struct{}))
	if existing {
		return 0, fmt.Errorf("a ping to this node is already in progress")
	}
	defer r.pings.Delete(dst.String())
	ch := v.(chan struct{})
	select {
	case <-ctx.Done():
		return 0, fmt.Errorf("ping timed out")
	case <-ch:
		return time.Since(start), nil
	}
}
