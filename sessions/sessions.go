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
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/types"
	"github.com/neilalexander/utp"
	"go.uber.org/atomic"
)

type Sessions struct {
	r             *router.Router
	log           *log.Logger                // logger
	context       context.Context            // router context
	cancel        context.CancelFunc         // shut down the router
	streams       chan net.Conn              // accepted connections
	sessions      map[net.Addr]*atomic.Int32 // open sessions
	sessionsMutex sync.RWMutex               // protects sessions
	tlsCert       *tls.Certificate           //
	tlsServerCfg  *tls.Config                //
	utpSocket     *utp.Socket                //
}

func NewSessions(log *log.Logger, r *router.Router) *Sessions {
	ctx, cancel := context.WithCancel(context.Background())
	s, err := utp.NewSocketFromPacketConnNoClose(r)
	if err != nil {
		panic(fmt.Errorf("utp.NewSocketFromPacketConnNoClose: %w", err))
	}
	q := &Sessions{
		r:         r,
		log:       log,
		context:   ctx,
		cancel:    cancel,
		streams:   make(chan net.Conn, 16),
		sessions:  make(map[net.Addr]*atomic.Int32),
		utpSocket: s,
	}

	s.OnAttach(func(remote net.Addr) {
		q.sessionsMutex.Lock()
		defer q.sessionsMutex.Unlock()
		if _, ok := q.sessions[remote]; !ok {
			q.sessions[remote] = &atomic.Int32{}
		}
		q.sessions[remote].Inc()
	})
	s.OnDetach(func(remote net.Addr) {
		q.sessionsMutex.Lock()
		defer q.sessionsMutex.Unlock()
		if _, ok := q.sessions[remote]; !ok {
			return
		}
		if q.sessions[remote].Dec() == 0 {
			delete(q.sessions, remote)
		}
	})

	q.tlsCert = q.generateTLSCertificate()
	q.tlsServerCfg = &tls.Config{
		Certificates: []tls.Certificate{*q.tlsCert},
		ClientAuth:   tls.RequireAnyClientCert,
	}

	go q.listener()
	return q
}

func (q *Sessions) Sessions() []ed25519.PublicKey {
	var sessions []ed25519.PublicKey
	q.sessionsMutex.RLock()
	defer q.sessionsMutex.RUnlock()
	for s := range q.sessions {
		switch k := s.(type) {
		case types.PublicKey:
			sessions = append(sessions, k[:])
		default:
		}
	}
	return sessions
}

func (q *Sessions) generateTLSCertificate() *tls.Certificate {
	private, public := q.r.PrivateKey(), q.r.PublicKey()
	id := hex.EncodeToString(public[:])

	template := x509.Certificate{
		Subject: pkix.Name{
			CommonName: id,
		},
		SerialNumber: big.NewInt(1),
		NotAfter:     time.Now().Add(time.Hour * 24 * 365),
		DNSNames:     []string{id},
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		ed25519.PublicKey(public[:]),
		ed25519.PrivateKey(private[:]),
	)
	if err != nil {
		panic(fmt.Errorf("x509.CreateCertificate: %w", err))
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(ed25519.PrivateKey(private[:]))
	if err != nil {
		panic(fmt.Errorf("x509.MarshalPKCS8PrivateKey: %w", err))
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(fmt.Errorf("tls.X509KeyPair: %w", err))
	}

	return &tlsCert
}
