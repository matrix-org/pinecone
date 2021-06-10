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

type SwitchAnnouncement struct {
	RootPublicKey PublicKey
	Sequence      Varu64
	Signatures    []SignatureWithHop
}

func (a *SwitchAnnouncement) Sign(privKey ed25519.PrivateKey, forPort SwitchPortID) error {
	var body [65535]byte
	n, err := a.MarshalBinary(body[:])
	if err != nil {
		return fmt.Errorf("a.MarshalBinary: %w", err)
	}
	hop := SignatureWithHop{
		Hop: Varu64(forPort),
	}
	copy(hop.PublicKey[:], privKey.Public().(ed25519.PublicKey))
	if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
		copy(hop.Signature[:], ed25519.Sign(privKey, body[:n]))
	}
	a.Signatures = append(a.Signatures, hop)
	return nil
}

func (a *SwitchAnnouncement) UnmarshalBinary(data []byte) (int, error) {
	expected := ed25519.PublicKeySize + 1
	if size := len(data); size < expected {
		return 0, fmt.Errorf("expecting at least %d bytes, got %d bytes", expected, size)
	}
	remaining := data[copy(a.RootPublicKey[:ed25519.PublicKeySize], data):]
	if err := a.Sequence.UnmarshalBinary(remaining); err != nil {
		return 0, fmt.Errorf("a.Sequence.UnmarshalBinary: %w", err)
	}
	remaining = remaining[a.Sequence.Length():]
	for i := Varu64(0); len(remaining) >= ed25519.PublicKeySize+ed25519.SignatureSize+1; i++ {
		var signature SignatureWithHop
		n, err := signature.UnmarshalBinary(remaining[:])
		if err != nil {
			return 0, fmt.Errorf("signature.UnmarshalBinary: %w", err)
		}
		if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
			if !ed25519.Verify(signature.PublicKey[:], data[:len(data)-len(remaining)], signature.Signature[:]) {
				return 0, fmt.Errorf("signature verification failed for hop %d", signature.Hop)
			}
		}
		a.Signatures = append(a.Signatures, signature)
		remaining = remaining[n:]
	}
	return len(data) - len(remaining), nil
}

func (a *SwitchAnnouncement) MarshalBinary(buffer []byte) (int, error) {
	seq, err := a.Sequence.MarshalBinary()
	if err != nil {
		return 0, fmt.Errorf("a.Sequence.MarshalBinary: %w", err)
	}
	offset := 0
	offset += copy(buffer[offset:], a.RootPublicKey[:]) // a.RootPublicKey
	offset += copy(buffer[offset:], seq)                // a.Sequence
	for _, sig := range a.Signatures {
		n, err := sig.MarshalBinary(buffer[offset:])
		if err != nil {
			return 0, fmt.Errorf("sig.MarshalBinary: %w", err)
		}
		offset += n
	}
	return offset, nil
}

func (a *SwitchAnnouncement) Coords() SwitchPorts {
	sigs := a.Signatures
	coords := make(SwitchPorts, 0, len(sigs))
	for _, sig := range sigs {
		coords = append(coords, SwitchPortID(sig.Hop))
	}
	return coords
}

func (a *SwitchAnnouncement) PeerCoords(public PublicKey) (SwitchPorts, error) {
	sigs := a.Signatures
	last := len(sigs) - 1
	if sigs[last].PublicKey != public {
		return nil, fmt.Errorf("invalid last hop")
	}
	coords := make(SwitchPorts, 0, len(sigs))
	for _, sig := range sigs[:last] {
		coords = append(coords, SwitchPortID(sig.Hop))
	}
	return coords, nil
}
