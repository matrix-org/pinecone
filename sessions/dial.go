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

package sessions

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"

	"github.com/matrix-org/pinecone/types"
)

// DialContext dials a given public key using the supplied network.
// The network field can be used to specify which routing algorithm to
// use for the session: "ed25519+greedy" for greedy routing or "ed25519+source"
// for source routing - DHT lookups and pathfinds will be performed for these
// networks automatically. Otherwise, the default "ed25519" will use snake
// routing. The address must be the destination public key specified in hex.
// If the context expires then the session will be torn down automatically.
func (q *Sessions) DialContext(ctx context.Context, network, addrstr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addrstr)
	if err != nil {
		return nil, fmt.Errorf("net.SplitHostPort: %w", err)
	}

	pk := make(ed25519.PublicKey, ed25519.PublicKeySize)
	pkb, err := hex.DecodeString(host)
	if err != nil {
		return nil, fmt.Errorf("hex.DecodeString: %w", err)
	}
	if len(pkb) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("host must be length of an ed25519 public key")
	}
	copy(pk, pkb)

	var addr net.Addr
	switch network {
	case "ed25519+greedy":
		_, addr, err = q.r.DHTSearch(ctx, pk, types.FullMask[:], false)
		if err != nil {
			return nil, fmt.Errorf("q.dht.search: %w", err)
		}

	case "ed25519+source":
		_, coords, err := q.r.DHTSearch(ctx, pk, types.FullMask[:], false)
		if err != nil {
			return nil, fmt.Errorf("q.dht.search: %w", err)
		}
		addr, err = q.r.Pathfind(ctx, coords)
		if err != nil {
			return nil, fmt.Errorf("q.pathfinder.pathfind: %w", err)
		}

	case "ed25519":
		fallthrough

	default:
		a := types.PublicKey{}
		copy(a[:], pk)
		addr = a
	}

	session, err := q.utpSocket.DialAddrContext(
		ctx, addr,
	)
	if err != nil {
		return nil, fmt.Errorf("q.utpSocket.DialContext: %w", err)
	}

	session = tls.Client(session, &tls.Config{
		InsecureSkipVerify: true,
		GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return q.tlsCert, nil
		},
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if c := len(rawCerts); c != 1 {
				return fmt.Errorf("expected exactly one peer certificate but got %d", c)
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("x509.ParseCertificate: %w", err)
			}
			public, ok := cert.PublicKey.(ed25519.PublicKey)
			if !ok {
				return fmt.Errorf("expected ed25519 public key")
			}
			if !bytes.Equal(public, pk) {
				return fmt.Errorf("remote side returned incorrect public key")
			}
			return nil
		},
	})

	return session, nil
}

// Dial dials a given public key using the supplied network.
// The network field can be used to specify which routing algorithm to
// use for the session: "ed25519+greedy" for greedy routing or "ed25519+source"
// for source routing. DHT lookups and pathfinds will be performed for these
// networks automatically. Otherwise, the default "ed25519" will use snake
// routing. The address must be the destination public key specified in hex.
func (q *Sessions) Dial(network, addr string) (net.Conn, error) {
	return q.DialContext(context.Background(), network, addr)
}

// DialTLS is an alias for Dial, as all sessions are TLS-encrypted.
func (q *Sessions) DialTLS(network, addr string) (net.Conn, error) {
	return q.DialTLSContext(context.Background(), network, addr)
}

// DialTLSContext is an alias for DialContext, as all sessions are
// TLS-encrypted.
func (q *Sessions) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return q.DialContext(ctx, network, addr)
}
