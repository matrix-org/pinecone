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

package types

import (
	"crypto/ed25519"
	"fmt"
)

type WakeupBroadcast struct {
	Sequence Varu64
	Root
	Signature [ed25519.SignatureSize]byte
}

func (w *WakeupBroadcast) ProtectedPayload() ([]byte, error) {
	buffer := make([]byte, w.Sequence.Length()+w.Root.Length())
	offset := 0
	n, err := w.Sequence.MarshalBinary(buffer[:])
	if err != nil {
		return nil, fmt.Errorf("w.Sequence.MarshalBinary: %w", err)
	}
	offset += n
	offset += copy(buffer[offset:], w.RootPublicKey[:])
	n, err = w.RootSequence.MarshalBinary(buffer[offset:])
	if err != nil {
		return nil, fmt.Errorf("w.RootSequence.MarshalBinary: %w", err)
	}
	offset += n
	return buffer[:offset], nil
}

func (w *WakeupBroadcast) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < w.Sequence.Length()+w.Root.Length()+ed25519.SignatureSize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	n, err := w.Sequence.MarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("w.Sequence.MarshalBinary: %w", err)
	}
	offset += n
	offset += copy(buf[offset:], w.RootPublicKey[:])
	n, err = w.RootSequence.MarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("w.RootSequence.MarshalBinary: %w", err)
	}
	offset += n
	offset += copy(buf[offset:], w.Signature[:])
	return offset, nil
}

func (w *WakeupBroadcast) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < w.Sequence.MinLength()+w.Root.MinLength()+ed25519.SignatureSize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	n, err := w.Sequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("w.Sequence.UnmarshalBinary: %w", err)
	}
	offset += n
	offset += copy(w.RootPublicKey[:], buf[offset:])
	n, err = w.RootSequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("w.RootSequence.UnmarshalBinary: %w", err)
	}
	offset += n
	offset += copy(w.Signature[:], buf[offset:])
	return offset, nil
}
