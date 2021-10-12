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
)

type SignatureWithHop struct {
	Hop       Varu64
	PublicKey PublicKey
	Signature Signature
}

const SignatureWithHopMinSize = ed25519.PublicKeySize + ed25519.SignatureSize + 1

func (a *SignatureWithHop) UnmarshalBinary(data []byte) (int, error) {
	if size := len(data); size < SignatureWithHopMinSize {
		return 0, fmt.Errorf("SignatureWithHop expects at least %d bytes, got %d bytes", SignatureWithHopMinSize, size)
	}
	l, err := a.Hop.UnmarshalBinary(data)
	if err != nil {
		return 0, fmt.Errorf("a.Hop.UnmarshalBinary: %w", err)
	}
	offset := l
	remaining := data[offset:]
	remaining = remaining[copy(a.PublicKey[:], remaining):]
	remaining = remaining[copy(a.Signature[:], remaining):]
	return len(data) - len(remaining), nil
}

func (a *SignatureWithHop) MarshalBinary(data []byte) (int, error) {
	offset, err := a.Hop.MarshalBinary(data[:])
	if err != nil {
		return 0, fmt.Errorf("a.Hop.MarshalBinary: %w", err)
	}
	if len(data) < SignatureWithHopMinSize {
		return 0, fmt.Errorf("buffer is not big enough (must be %d bytes)", SignatureWithHopMinSize)
	}
	offset += copy(data[offset:offset+ed25519.PublicKeySize], a.PublicKey[:])
	offset += copy(data[offset:offset+ed25519.SignatureSize], a.Signature[:])
	return offset, nil
}
