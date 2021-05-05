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
	"crypto/tls"
	"fmt"
	"net"
)

func (q *Sessions) listener() {
	q.log.Println("Listening for UTP sessions")
	for {
		session, err := q.utpSocket.Accept()
		if err != nil {
			q.log.Println("Failed to accept UTP:", err)
			return
		}

		go func(session net.Conn) {
			q.streams <- session
		}(session)
	}
}

// Accept blocks until a new session request is received. The
// connection returned by this function will be TLS-encrypted.
func (q *Sessions) Accept() (net.Conn, error) {
	stream := <-q.streams
	if stream == nil {
		return nil, fmt.Errorf("listener closed")
	}
	stream = tls.Server(stream, q.tlsServerCfg)
	return stream, nil
}

func (q *Sessions) Addr() net.Addr {
	return q.r.Addr()
}

func (q *Sessions) Close() error {
	q.cancel()
	return nil
}
