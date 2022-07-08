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
	"bytes"
	"crypto/ed25519"
	"fmt"
	"testing"
)

func TestMarshalUnmarshalBootstrap(t *testing.T) {
	pkr, _, _ := ed25519.GenerateKey(nil)
	_, sk1, _ := ed25519.GenerateKey(nil)
	input := &VirtualSnakeBootstrap{
		Sequence: 7,
		Root: Root{
			RootSequence: 1,
		},
	}
	copy(input.RootPublicKey[:], pkr)
	var err error
	protected, err := input.ProtectedPayload()
	if err != nil {
		t.Fatal(err)
	}
	copy(
		input.Signature[:],
		ed25519.Sign(sk1[:], protected),
	)
	var buffer [65535]byte
	n, err := input.MarshalBinary(buffer[:])
	if err != nil {
		t.Fatal(err)
	}

	var output VirtualSnakeBootstrap
	if _, err = output.UnmarshalBinary(buffer[:n]); err != nil {
		t.Fatal(err)
	}

	if output.Sequence != input.Sequence {
		fmt.Println("expected:", input.Sequence)
		fmt.Println("got:", output.Sequence)
		t.Fatalf("bootstrap sequence doesn't match")
	}
	if !bytes.Equal(pkr, output.RootPublicKey[:]) {
		fmt.Println("expected:", pkr)
		fmt.Println("got:", output.RootPublicKey)
		t.Fatalf("root public key doesn't match")
	}
	if output.RootSequence != input.RootSequence {
		fmt.Println("expected:", input.RootSequence)
		fmt.Println("got:", output.RootSequence)
		t.Fatalf("root sequence doesn't match")
	}
	if !bytes.Equal(input.Signature[:], output.Signature[:]) {
		fmt.Println("expected:", input.Signature)
		fmt.Println("got:", output.Signature)
		t.Fatalf("root public key doesn't match")
	}
}
