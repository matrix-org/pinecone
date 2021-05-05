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

package types

import (
	"crypto/ed25519"
	"fmt"
	"os"
)

type Pathfind struct {
	Boundary   uint8
	VsetPath   bool
	Signatures []SignatureWithHop
}

func (p Pathfind) Sign(privKey ed25519.PrivateKey, forPort SwitchPortID) (*Pathfind, error) {
	var body [65535]byte
	n, err := p.MarshalBinary(body[:])
	if err != nil {
		return nil, fmt.Errorf("a.MarshalBinary: %w", err)
	}
	hop := SignatureWithHop{
		Hop: Varu64(forPort),
	}
	copy(hop.PublicKey[:], privKey.Public().(ed25519.PublicKey))
	if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
		copy(hop.Signature[:], ed25519.Sign(privKey, body[1:n]))
	}
	p.Signatures = append(p.Signatures, hop)
	return &p, nil
}

func (p *Pathfind) ReturnPath(reverse bool) SwitchPorts {
	signatures := p.Signatures
	if len(signatures) < int(p.Boundary) {
		return SwitchPorts{}
	}
	if reverse {
		signatures = signatures[p.Boundary:]
	} else {
		signatures = signatures[:p.Boundary-1]
	}
	count := len(signatures)
	path := make(SwitchPorts, count)
	if reverse {
		for i, sig := range signatures {
			path[count-1-i] = SwitchPortID(sig.Hop)
		}
	} else {
		for i, sig := range signatures {
			path[i] = SwitchPortID(sig.Hop)
		}
	}
	return path
}

func (p Pathfind) MarshalBinary(buf []byte) (int, error) {
	offset := copy(buf, []byte{byte(p.Boundary)})
	if p.VsetPath {
		offset += copy(buf[offset:], []byte{1})
	} else {
		offset += copy(buf[offset:], []byte{0})
	}
	for _, sig := range p.Signatures {
		n, err := sig.MarshalBinary(buf[offset:])
		if err != nil {
			return 0, fmt.Errorf("a.MarshalBinary: %w", err)
		}
		if n < SignatureWithHopMinSize {
			return 0, fmt.Errorf("not enough signature bytes")
		}
		offset += n
	}
	return offset, nil
}

func (p *Pathfind) UnmarshalBinary(b []byte) (int, error) {
	if len(b) == 0 {
		p.Signatures = []SignatureWithHop{}
		return 0, nil
	}
	p.Boundary = uint8(b[0])
	if b[1] != 0 {
		p.VsetPath = true
	}
	remaining := b[2:]
	for i := Varu64(0); len(remaining) >= ed25519.PublicKeySize+ed25519.SignatureSize+1; i++ {
		var signature SignatureWithHop
		n, err := signature.UnmarshalBinary(remaining[:])
		if err != nil {
			return 0, fmt.Errorf("signature.UnmarshalBinary: %w", err)
		}
		if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
			if !ed25519.Verify(signature.PublicKey[:], b[1:len(b)-len(remaining)], signature.Signature[:]) {
				return 0, fmt.Errorf("signature verification failed")
			}
		}
		p.Signatures = append(p.Signatures, signature)
		remaining = remaining[n:]
	}
	return len(b) - len(remaining), nil
}
