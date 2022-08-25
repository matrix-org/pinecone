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
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

type PublicKey [ed25519.PublicKeySize]byte
type PrivateKey [ed25519.PrivateKeySize]byte
type Signature [ed25519.SignatureSize]byte

func (a PrivateKey) Public() PublicKey {
	var public PublicKey
	ed := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	copy(ed, a[:])
	copy(public[:], ed.Public().(ed25519.PublicKey))
	return public
}

var FullMask = PublicKey{
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
}

func (a PublicKey) IsEmpty() bool {
	empty := PublicKey{}
	return a == empty
}

func (a PublicKey) EqualMaskTo(b, m PublicKey) bool {
	for i := range a {
		if (a[i] & m[i]) != (b[i] & m[i]) {
			return false
		}
	}
	return true
}

func (a PublicKey) CompareTo(b PublicKey) int {
	return bytes.Compare(a[:], b[:])
}

func (a PublicKey) String() string {
	return fmt.Sprintf("%v", hex.EncodeToString(a[:]))
}

func (a PublicKey) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

func (a PublicKey) Network() string {
	return "ed25519"
}
