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

type DHTQueryRequest struct {
	RequestID [8]byte
	PublicKey [ed25519.PublicKeySize]byte
}

func (r *DHTQueryRequest) MarshalBinary(buffer []byte) (int, error) {
	if len(buffer) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("not enough bytes")
	}
	offset := 0
	offset += copy(buffer[offset:], r.RequestID[:8])
	offset += copy(buffer[offset:], r.PublicKey[:ed25519.PublicKeySize])
	return offset, nil
}

func (r *DHTQueryRequest) UnmarshalBinary(buffer []byte) (int, error) {
	if len(buffer) < ed25519.PublicKeySize {
		return 0, fmt.Errorf("not enough bytes")
	}
	remaining := buffer[:]
	remaining = remaining[copy(r.RequestID[:], remaining):]
	copy(r.PublicKey[:], remaining)
	return len(buffer) - len(remaining), nil
}

type DHTQueryResponse struct {
	RequestID [8]byte
	Results   []DHTNode
	PublicKey [ed25519.PublicKeySize]byte
}

func (r *DHTQueryResponse) MarshalBinary(buffer []byte, private ed25519.PrivateKey) (int, error) {
	if len(buffer) < 8+1+ed25519.PublicKeySize+ed25519.SignatureSize {
		return 0, fmt.Errorf("not enough bytes")
	}
	offset := 0
	offset += copy(buffer[offset:], r.RequestID[:])
	offset += copy(buffer[offset:], []byte{byte(len(r.Results))})
	for _, result := range r.Results {
		n, err := result.MarshalBinary(buffer[offset:])
		if err != nil {
			return 0, fmt.Errorf("result.MarshalBinary: %w", err)
		}
		offset += n
	}
	offset += copy(buffer[offset:], r.PublicKey[:])
	if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
		offset += copy(buffer[offset:], ed25519.Sign(private, buffer[:offset]))
	} else {
		offset += ed25519.SignatureSize
	}
	return offset, nil
}

func (r *DHTQueryResponse) UnmarshalBinary(buffer []byte) (int, error) {
	if len(buffer) < 8+1 {
		return 0, fmt.Errorf("not enough bytes")
	}
	remaining := buffer[:]
	remaining = remaining[copy(r.RequestID[:], remaining):]
	responses := int(remaining[0])
	remaining = remaining[1:]

	for i := 0; i < responses; i++ {
		var result DHTNode
		n, err := result.UnmarshalBinary(remaining)
		if err != nil {
			return 0, fmt.Errorf("response.UnmarshalBinary: %w", err)
		}
		r.Results = append(r.Results, result)
		remaining = remaining[n:]
	}
	remaining = remaining[copy(r.PublicKey[:], remaining):]
	boundary := len(buffer) - len(remaining)
	if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
		if !ed25519.Verify(r.PublicKey[:], buffer[:boundary], remaining[:ed25519.SignatureSize]) {
			return 0, fmt.Errorf("signature verification failed")
		}
	}
	return len(buffer) - len(remaining), nil
}

type DHTNode struct {
	PublicKey   PublicKey
	Coordinates SwitchPorts
}

func (n *DHTNode) MarshalBinary(buffer []byte) (int, error) {
	coords, err := n.Coordinates.MarshalBinary()
	if err != nil {
		return 0, fmt.Errorf("n.Coordinates.MarshalBinary: %w", err)
	}
	if len(buffer) < ed25519.PublicKeySize+len(coords) {
		return 0, fmt.Errorf("not enough bytes")
	}
	offset := 0
	offset += copy(buffer[offset:], n.PublicKey[:])
	offset += copy(buffer[offset:], coords)
	return offset, nil
}

func (n *DHTNode) UnmarshalBinary(buffer []byte) (int, error) {
	if len(buffer) < ed25519.PublicKeySize+1 {
		return 0, fmt.Errorf("not enough bytes")
	}
	remaining := buffer[:]
	remaining = remaining[copy(n.PublicKey[:], remaining[:ed25519.PublicKeySize]):]
	l, err := n.Coordinates.UnmarshalBinary(remaining)
	if err != nil {
		return 0, fmt.Errorf("n.Coordinates.UnmarshalBinary: %w", err)
	}
	remaining = remaining[l:]
	return len(buffer) - len(remaining), nil
}
