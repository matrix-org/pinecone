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

package simulator

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"

	"github.com/matrix-org/pinecone/types"
)

type PingType uint8

const (
	Ping PingType = iota
	Pong
)

const pingPreamble = "pineping"
const pingSize = len(pingPreamble) + (ed25519.PublicKeySize * 2) + 3

type PingPayload struct {
	pingType    PingType
	origin      types.PublicKey
	destination types.PublicKey
	hops        uint16
}

func (p *PingPayload) MarshalBinary(buffer []byte) (int, error) {
	if len(buffer) < pingSize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := copy(buffer, []byte(pingPreamble))
	buffer[offset] = uint8(p.pingType)
	offset++
	binary.BigEndian.PutUint16(buffer[offset:offset+2], p.hops)
	offset += 2
	offset += copy(buffer[offset:], p.origin[:ed25519.PublicKeySize])
	offset += copy(buffer[offset:], p.destination[:ed25519.PublicKeySize])
	return offset, nil
}

func (p *PingPayload) UnmarshalBinary(buffer []byte) (int, error) {
	if len(buffer) < pingSize {
		return 0, fmt.Errorf("buffer too small")
	}
	if string(buffer[:len(pingPreamble)]) != pingPreamble {
		return 0, fmt.Errorf("not a ping")
	}
	offset := len(pingPreamble)
	p.pingType = PingType(buffer[offset])
	offset++
	p.hops = binary.BigEndian.Uint16(buffer[offset : offset+2])
	offset += 2
	offset += copy(p.origin[:], buffer[offset:])
	offset += copy(p.destination[:], buffer[offset:])
	return offset, nil
}
