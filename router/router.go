package router

import (
	"context"
	"crypto/ed25519"
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
const TrafficBuffer = 128

type Router struct {
	phony.Inbox
	log       *log.Logger
	id        string
	simulator Simulator
	context   context.Context
	cancel    context.CancelFunc
	public    types.PublicKey
	private   types.PrivateKey
	active    sync.Map
	pings     sync.Map // types.PublicKey -> chan struct{}
	local     *peer
	state     *state
	_peers    []*peer
}

func NewRouter(log *log.Logger, sk ed25519.PrivateKey, id string, sim Simulator) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Router{
		log:       log,
		id:        id,
		simulator: sim,
		context:   ctx,
		cancel:    cancel,
		_peers:    make([]*peer, PortCount),
	}
	copy(r.private[:], sk)
	r.public = r.private.Public()
	r.state = &state{
		r:      r,
		_table: make(virtualSnakeTable),
	}
	r.local = r.localPeer()
	r._peers[0] = r.local
	r.state.Act(nil, r.state._start)
	r.log.Println("Router identity:", r.public.String())
	return r
}

func (r *Router) peers() []*peer {
	var peers []*peer
	phony.Block(r, func() {
		peers = r._peers
	})
	return peers
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
	phony.Block(r, func() {
		for i, p := range r._peers {
			if i == 0 {
				// Port 0 is reserved for the local router.
				continue
			}
			if p != nil {
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
			r._peers[i] = new
			r.log.Println("Connected to peer", new.public.String(), "on port", new.port)
			v, _ := r.active.LoadOrStore(hex.EncodeToString(new.public[:])+zone, atomic.NewUint64(0))
			v.(*atomic.Uint64).Inc()
			new.reader.Act(nil, new._read)
			new.writer.Act(nil, new._write)
			r.state.Act(nil, func() {
				r.state._sendTreeAnnouncementToPeer(r.state._rootAnnouncement(), new)
			})
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
		ourCapabilities,
		0, // unused
		0, // unused
	}
	handshake = append(handshake, r.public[:ed25519.PublicKeySize]...)
	handshake = append(handshake, ed25519.Sign(r.private[:], handshake)...)
	//_ = conn.SetWriteDeadline(time.Now().Add(PeerKeepaliveInterval))
	if _, err := conn.Write(handshake); err != nil {
		conn.Close()
		return 0, fmt.Errorf("conn.Write: %w", err)
	}
	//_ = conn.SetWriteDeadline(time.Time{})
	//_ = conn.SetReadDeadline(time.Now().Add(PeerKeepaliveInterval))
	if _, err := io.ReadFull(conn, handshake); err != nil {
		conn.Close()
		return 0, fmt.Errorf("io.ReadFull: %w", err)
	}
	//_ = conn.SetReadDeadline(time.Time{})
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
		return 0, fmt.Errorf("r.Connect failed: %w (close: %s)", err, conn.Close())
	}
	return port, err
}

func (r *Router) SNEKPing(ctx context.Context, dst types.PublicKey) (time.Duration, error) {
	r.local.reader.Act(nil, func() {
		_ = r.local._receive(r.local, &types.Frame{
			Type:           types.TypeSNEKPing,
			DestinationKey: dst,
			SourceKey:      r.public,
		})
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
	r.local.reader.Act(nil, func() {
		_ = r.local._receive(r.local, &types.Frame{
			Type:        types.TypeTreePing,
			Destination: dst,
			Source:      r.state.coords(),
		})
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
