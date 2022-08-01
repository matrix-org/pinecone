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
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/cloudflare/circl/sign/eddilithium2"
	"github.com/lucas-clemente/quic-go"
	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/types"
)

type Sessions struct {
	r            *router.Router
	log          types.Logger                // logger
	context      context.Context             // router context
	cancel       context.CancelFunc          // shut down the router
	protocols    map[string]*SessionProtocol // accepted connections by proto
	tlsCert      *tls.Certificate            //
	tlsServerCfg *tls.Config                 //
	quicListener quic.Listener               //
	quicConfig   *quic.Config                //
}

type SessionProtocol struct {
	s        *Sessions
	proto    string
	streams  chan net.Conn
	sessions sync.Map // types.PublicKey -> *activeSession
}

type activeSession struct {
	quic.Session
	sync.RWMutex
}

func NewSessions(log types.Logger, r *router.Router, protos []string) *Sessions {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Sessions{
		r:         r,
		log:       log,
		context:   ctx,
		cancel:    cancel,
		protocols: make(map[string]*SessionProtocol, len(protos)),
		quicConfig: &quic.Config{
			MaxIdleTimeout:          time.Second * 15,
			DisablePathMTUDiscovery: true,
		},
	}
	for _, proto := range protos {
		s.protocols[proto] = &SessionProtocol{
			s:       s,
			proto:   proto,
			streams: make(chan net.Conn, 1),
		}
	}

	s.tlsCert = s.generateTLSCertificate()
	s.tlsServerCfg = &tls.Config{
		Certificates: []tls.Certificate{*s.tlsCert},
		ClientAuth:   tls.RequireAnyClientCert,
		NextProtos:   protos,
	}

	var err error
	s.quicListener, err = quic.Listen(r, s.tlsServerCfg, s.quicConfig)
	if err != nil {
		panic(fmt.Errorf("utp.NewSocketFromPacketConnNoClose: %w", err))
	}

	go s.listener()
	return s
}

func (s *Sessions) Close() error {
	s.cancel()
	return nil
}

func (s *Sessions) Protocol(proto string) *SessionProtocol {
	return s.protocols[proto]
}

func (s *SessionProtocol) Sessions() []eddilithium2.PublicKey {
	var sessions []eddilithium2.PublicKey
	s.sessions.Range(func(k, _ interface{}) bool {
		switch pk := k.(type) {
		case types.PublicKey:
			pkTmp := eddilithium2.PublicKey{}
			pkTmp.UnmarshalBinary(pk[:])
			sessions = append(sessions, pkTmp)
		default:
		}
		return true
	})
	return sessions
}

func (p *SessionProtocol) getSession(pk types.PublicKey) (*activeSession, bool) {
	v, ok := p.sessions.LoadOrStore(pk, &activeSession{})
	return v.(*activeSession), ok
}

func (s *Sessions) generateTLSCertificate() *tls.Certificate {
	privateTmp, publicTmp := s.r.PrivateKey(), s.r.PublicKey()
	id := hex.EncodeToString(publicTmp[:])

	template := x509.Certificate{
		Subject: pkix.Name{
			CommonName: id,
		},
		SerialNumber: big.NewInt(1),
		NotAfter:     time.Now().Add(time.Hour * 24 * 365),
		DNSNames:     []string{id},
	}

	public := eddilithium2.PublicKey{}
	private := eddilithium2.PrivateKey{}

	public.UnmarshalBinary(publicTmp[:])
	private.UnmarshalBinary(privateTmp[:])

	certDER, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		public,
		private,
	)
	if err != nil {
		panic(fmt.Errorf("x509.CreateCertificate: %w", err))
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(eddilithium2.PrivateKey(private))
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
