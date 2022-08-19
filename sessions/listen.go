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
	"crypto/ed25519"
	"fmt"
	"net"

	"github.com/matrix-org/pinecone/types"
)

func (q *Sessions) listener() {
	for {
		con, err := q.quicListener.Accept(q.context)
		if err != nil {
			return
		}

		key := con.RemoteAddr().(types.PublicKey)
		tls := con.ConnectionState().TLS
		if c := len(tls.PeerCertificates); c != 1 {
			continue
		}
		cert := tls.PeerCertificates[0]
		public, ok := cert.PublicKey.(ed25519.PublicKey)
		if !ok {
			continue
		}
		if !bytes.Equal(public, key[:]) {
			continue
		}

		if proto := q.Protocol(con.ConnectionState().TLS.NegotiatedProtocol); proto != nil {
			entry, ok := proto.getSession(key)
			entry.Lock()
			if ok {
				_ = con.CloseWithError(0, "connection replaced")
			}
			entry.Connection = con
			entry.Unlock()
			go proto.sessionlistener(entry)
		}
	}
}

func (s *SessionProtocol) sessionlistener(session *activeSession) {
	key, ok := session.RemoteAddr().(types.PublicKey)
	if !ok {
		return
	}

	defer s.sessions.Delete(key)

	ctx := session.Context()
	for {
		stream, err := session.AcceptStream(ctx)
		if err != nil {
			return
		}

		select {
		case <-ctx.Done():
		case s.streams <- &Stream{stream, session}:
		}
	}
}

// Accept blocks until a new connection request is received. The
// connection returned by this function will be TLS-encrypted.
func (s *SessionProtocol) Accept() (net.Conn, error) {
	stream := <-s.streams
	if stream == nil {
		return nil, fmt.Errorf("listener closed")
	}
	return stream, nil
}

func (s *SessionProtocol) Addr() net.Addr {
	return s.s.r.Addr()
}

func (s *SessionProtocol) Close() error {
	var err error = nil
	s.closeOnce.Do(func() {
		close(s.streams)
		err = s.s.quicListener.Close()
	})
	return err
}
