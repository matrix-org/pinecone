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

type BootstrapSignature struct {
	PublicKey PublicKey
	Signature Signature
}

const BootstrapSignatureSize = ed25519.PublicKeySize + ed25519.SignatureSize

func (s *BootstrapSignature) UnmarshalBinary(data []byte) (int, error) {
	if size := len(data); size < BootstrapSignatureSize {
		return 0, fmt.Errorf("BootstrapSignature expects at least %d bytes, got %d bytes", BootstrapSignatureSize, size)
	}
	offset := 0
	remaining := data[offset:]
	remaining = remaining[copy(s.PublicKey[:], remaining):]
	remaining = remaining[copy(s.Signature[:], remaining):]
	return len(data) - len(remaining), nil
}

func (s *BootstrapSignature) MarshalBinary(data []byte) (int, error) {
	if len(data) < BootstrapSignatureSize {
		return 0, fmt.Errorf("buffer is not big enough (must be %d bytes)", BootstrapSignatureSize)
	}
	offset := 0
	offset += copy(data[offset:offset+ed25519.PublicKeySize], s.PublicKey[:])
	offset += copy(data[offset:offset+ed25519.SignatureSize], s.Signature[:])
	return offset, nil
}
