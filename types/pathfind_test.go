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
	"testing"
)

func TestMarshalUnmarshalPathfind(t *testing.T) {
	_, sk1, _ := ed25519.GenerateKey(nil)
	_, sk2, _ := ed25519.GenerateKey(nil)
	_, sk3, _ := ed25519.GenerateKey(nil)
	input := &Pathfind{
		Boundary: 1,
	}
	var err error
	input, err = input.Sign(sk1, 1)
	if err != nil {
		t.Fatal(err)
	}
	input, err = input.Sign(sk2, 2)
	if err != nil {
		t.Fatal(err)
	}
	input, err = input.Sign(sk3, 3)
	if err != nil {
		t.Fatal(err)
	}
	var buffer [65535]byte
	n, err := input.MarshalBinary(buffer[:])
	if err != nil {
		t.Fatal(err)
	}
	var output Pathfind
	if _, err = output.UnmarshalBinary(buffer[:n]); err != nil {
		t.Fatal(err)
	}
}
