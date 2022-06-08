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
	"net"

	"github.com/matrix-org/pinecone/types"
)

type PingType uint8

const (
	SNEKPing PingType = iota
	SNEKPong
)

type PingPayload struct {
	pingType    PingType
	origin      net.Addr
	destination net.Addr
	hops        uint16
}

func (p *PingPayload) MarshalBinary(buffer []byte) (int, error) {
	offset := 0
	buffer[offset] = byte(p.pingType)
	offset += 1
	binary.BigEndian.PutUint16(buffer[offset:offset+2], p.hops)
	offset += 2

	switch orig := p.origin.(type) {
	case types.PublicKey:
		offset += copy(buffer[offset:], orig[:ed25519.PublicKeySize])
	default:
		return 0, fmt.Errorf("unknown address type")
	}

	switch dest := p.destination.(type) {
	case types.PublicKey:
		offset += copy(buffer[offset:], dest[:ed25519.PublicKeySize])
	default:
		return 0, fmt.Errorf("unknown address type")
	}

	return offset, nil
}

func (p *PingPayload) UnmarshalBinary(data []byte) (int, error) {
	offset := 0
	p.pingType = PingType(data[0])
	p.hops = binary.BigEndian.Uint16(data[1:3])
	offset += 3

	switch p.pingType {
	case SNEKPing, SNEKPong:
		tempKey := types.PublicKey{}
		offset += copy(tempKey[:], data[offset:])
		p.origin = net.Addr(tempKey)

		tempKey = types.PublicKey{}
		offset += copy(tempKey[:], data[offset:])
		p.destination = net.Addr(tempKey)
	default:
		return 0, fmt.Errorf("received invalid ping type")
	}

	return offset, nil
}
