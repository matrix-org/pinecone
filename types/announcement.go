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

type Root struct {
	RootPublicKey PublicKey `json:"root_public_key"`
	RootSequence  Varu64    `json:"root_sequence"`
}

func (r *Root) Length() int {
	return ed25519.PublicKeySize + r.RootSequence.Length()
}

func (r *Root) MinLength() int {
	return ed25519.PublicKeySize + 1
}

type SwitchAnnouncement struct {
	Root
	Signatures []SignatureWithHop
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
	if l, err := a.RootSequence.UnmarshalBinary(remaining); err != nil {
		return 0, fmt.Errorf("a.Sequence.UnmarshalBinary: %w", err)
	} else {
		remaining = remaining[l:]
	}
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
	offset := 0
	offset += copy(buffer[offset:], a.RootPublicKey[:]) // a.RootPublicKey
	dn, err := a.RootSequence.MarshalBinary(buffer[offset:])
	if err != nil {
		return 0, fmt.Errorf("a.Sequence.MarshalBinary: %w", err)
	}
	offset += dn
	for _, sig := range a.Signatures {
		n, err := sig.MarshalBinary(buffer[offset:])
		if err != nil {
			return 0, fmt.Errorf("sig.MarshalBinary: %w", err)
		}
		offset += n
	}
	return offset, nil
}

func (a *SwitchAnnouncement) SanityCheck(from PublicKey) error {
	if len(a.Signatures) == 0 {
		return fmt.Errorf("update has no signatures")
	}
	sigs := make(map[PublicKey]struct{}, len(a.Signatures))
	for index, sig := range a.Signatures {
		if index == 0 && sig.PublicKey != a.RootPublicKey {
			return fmt.Errorf("update first signature doesn't match root key")
		}
		if sig.Hop == 0 {
			return fmt.Errorf("update contains invalid 0 hop")
		}
		if index == len(a.Signatures)-1 && from != sig.PublicKey {
			return fmt.Errorf("update last signature is not from direct peer")
		}
		if _, ok := sigs[sig.PublicKey]; ok {
			return fmt.Errorf("update contains routing loop")
		}
		sigs[sig.PublicKey] = struct{}{}
	}
	return nil
}

func (a *SwitchAnnouncement) Coords() Coordinates {
	sigs := a.Signatures
	coords := make(Coordinates, 0, len(sigs))
	for _, sig := range sigs {
		coords = append(coords, SwitchPortID(sig.Hop))
	}
	return coords
}

func (a *SwitchAnnouncement) PeerCoords() Coordinates {
	sigs := a.Signatures
	coords := make(Coordinates, 0, len(sigs)-1)
	for _, sig := range sigs[:len(sigs)-1] {
		coords = append(coords, SwitchPortID(sig.Hop))
	}
	return coords
}

func (a *SwitchAnnouncement) AncestorParent() PublicKey {
	if len(a.Signatures) < 2 {
		return a.RootPublicKey
	}
	return a.Signatures[len(a.Signatures)-2].PublicKey
}

func (a *Root) EqualTo(b *Root) bool {
	return a.RootPublicKey == b.RootPublicKey && a.RootSequence == b.RootSequence
}

func (a *SwitchAnnouncement) IsLoopOrChildOf(pk PublicKey) bool {
	m := map[PublicKey]struct{}{}
	for _, sig := range a.Signatures {
		if sig.PublicKey == pk {
			return true
		}
		if _, ok := m[sig.PublicKey]; ok {
			return true
		}
		m[sig.PublicKey] = struct{}{}
	}
	return false
}
