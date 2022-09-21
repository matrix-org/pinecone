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

	"github.com/matrix-org/pinecone/types"
)

type PingType uint8

const (
	Ping PingType = iota
	Pong
)

type PingPayload struct {
	pingType    PingType
	origin      types.PublicKey
	destination types.PublicKey
	hops        uint16
}

func (p *PingPayload) MarshalBinary(buffer []byte) (int, error) {
	buffer[0] = uint8(p.pingType)
	offset := 1
	binary.BigEndian.PutUint16(buffer[offset:offset+2], p.hops)
	offset += 2
	offset += copy(buffer[offset:], p.origin[:ed25519.PublicKeySize])
	offset += copy(buffer[offset:], p.destination[:ed25519.PublicKeySize])
	return offset, nil
}

func (p *PingPayload) UnmarshalBinary(data []byte) (int, error) {
	p.pingType = PingType(data[0])
	offset := 1
	p.hops = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	offset += copy(p.origin[:], data[offset:])
	offset += copy(p.destination[:], data[offset:])
	return offset, nil
}
