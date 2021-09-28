package router

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/Arceliar/phony"
	"github.com/matrix-org/pinecone/types"
)

const PortCount = 8
const TrafficBuffer = 128

type Router struct {
	phony.Inbox
	log     *log.Logger        // immutable
	context context.Context    // immutable
	cancel  context.CancelFunc // immutable
	public  types.PublicKey    // immutable
	private types.PrivateKey   // immutable
	state   *state             // actor
	_peers  []*peer
}

func NewRouter(log *log.Logger, sk ed25519.PrivateKey) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Router{
		log:     log,
		context: ctx,
		cancel:  cancel,
		_peers:  make([]*peer, PortCount),
	}
	copy(r.private[:], sk)
	r.public = r.private.Public()
	r.state = &state{
		r:      r,
		_table: make(virtualSnakeTable),
	}
	r.log.Println("Router identity:", r.public.String())
	return r
}

func (r *Router) Close() error {
	phony.Block(r, r.cancel)
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

func (r *Router) Connect(conn net.Conn, public types.PublicKey, zone string, peertype int) error {
	var err error
	phony.Block(r, func() {
		for i, p := range r._peers {
			if p == nil {
				ctx, cancel := context.WithCancel(r.context)
				r._peers[i] = &peer{
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
				r._peers[i].started.Store(true)
				r._peers[i].reader.Act(r, r._peers[i]._read)
				r._peers[i].writer.Act(r, r._peers[i]._write)
				r.log.Println("Connected to peer", public.String(), "on port", i)
				return
			}
		}
		err = fmt.Errorf("no free switch ports")
	})
	return err
}

func (r *Router) AuthenticatedConnect(conn net.Conn, zone string, peertype int) error {
	handshake := []byte{
		ourVersion,
		ourCapabilities,
		0, // unused
		0, // unused
	}
	handshake = append(handshake, r.public[:ed25519.PublicKeySize]...)
	handshake = append(handshake, ed25519.Sign(r.private[:], handshake)...)
	_ = conn.SetWriteDeadline(time.Now().Add(PeerKeepaliveInterval))
	if _, err := conn.Write(handshake); err != nil {
		conn.Close()
		return fmt.Errorf("conn.Write: %w", err)
	}
	_ = conn.SetWriteDeadline(time.Time{})
	_ = conn.SetReadDeadline(time.Now().Add(PeerKeepaliveInterval))
	if _, err := io.ReadFull(conn, handshake); err != nil {
		conn.Close()
		return fmt.Errorf("io.ReadFull: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	if theirVersion := handshake[0]; theirVersion != ourVersion {
		conn.Close()
		return fmt.Errorf("mismatched node version")
	}
	if theirCapabilities := handshake[1]; theirCapabilities&ourCapabilities != ourCapabilities {
		conn.Close()
		return fmt.Errorf("mismatched node capabilities")
	}
	var public types.PublicKey
	var signature types.Signature
	offset := 4
	offset += copy(public[:], handshake[offset:offset+ed25519.PublicKeySize])
	copy(signature[:], handshake[offset:offset+ed25519.SignatureSize])
	if !ed25519.Verify(public[:], handshake[:offset], signature[:]) {
		conn.Close()
		return fmt.Errorf("peer sent invalid signature")
	}
	if err := r.Connect(conn, public, zone, peertype); err != nil {
		conn.Close()
		return err
	}
	return nil
}
