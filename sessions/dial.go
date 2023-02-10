// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"github.com/quic-go/quic-go"
)

// DialContext dials a given public key using the supplied network.
// The network field can be used to specify which routing algorithm to
// use for the connection: "ed25519+greedy" for greedy routing or "ed25519+source"
// for source routing - DHT lookups and pathfinds will be performed for these
// networks automatically. Otherwise, the default "ed25519" will use snake
// routing. The address must be the destination public key specified in hex.
// If the context expires then the connection will be torn down automatically.
func (s *SessionProtocol) DialContext(ctx context.Context, network, addrstr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addrstr)
	if err != nil {
		return nil, fmt.Errorf("net.SplitHostPort: %w", err)
	}

	var pk types.PublicKey
	var addr net.Addr
	pkb, err := hex.DecodeString(host)
	if err != nil {
		return nil, fmt.Errorf("hex.DecodeString: %w", err)
	}
	if len(pkb) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("host must be length of an ed25519 public key")
	}
	copy(pk[:], pkb)
	addr = pk

	if pk == s.s.r.PublicKey() {
		return nil, fmt.Errorf("loopback dial")
	}

	var retrying bool
retry:
	session, ok := s.getSession(pk)
	if !ok {
		session.Lock()
		tlsConfig := &tls.Config{
			NextProtos:         []string{s.proto},
			InsecureSkipVerify: true,
			GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
				return s.s.tlsCert, nil
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
				if !bytes.Equal(public, pk[:]) {
					return fmt.Errorf("remote side returned incorrect public key")
				}
				return nil
			},
		}

		session.Connection, err = quic.DialContext(ctx, s.s.r, addr, addrstr, tlsConfig, s.s.quicConfig)
		session.Unlock()
		if err != nil {
			if err == context.DeadlineExceeded {
				return nil, err
			}
			return nil, fmt.Errorf("quic.Dial: %w", err)
		}

		go s.sessionlistener(session)
	} else {
		session.RLock()
		defer session.RUnlock()
	}

	if session.Connection == nil {
		s.sessions.Delete(pk)
		return nil, fmt.Errorf("connection failed to open")
	}

	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		s.sessions.Delete(pk)
		if !retrying {
			retrying = true
			goto retry
		}
		return nil, fmt.Errorf("connection.OpenStream: %w", err)
	}

	return &Stream{stream, session}, nil
}

// Dial dials a given public key using the supplied network.
// The address must be the destination public key specified in hex.
func (q *SessionProtocol) Dial(network, addr string) (net.Conn, error) {
	return q.DialContext(context.Background(), network, addr)
}

// DialTLS is an alias for Dial, as all sessions are TLS-encrypted.
func (q *SessionProtocol) DialTLS(network, addr string) (net.Conn, error) {
	return q.DialTLSContext(context.Background(), network, addr)
}

// DialTLSContext is an alias for DialContext, as all sessions are
// TLS-encrypted.
func (q *SessionProtocol) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return q.DialContext(ctx, network, addr)
}
