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
	"fmt"
	"testing"

	"github.com/cloudflare/circl/sign/eddilithium2"
)

func TestMarshalUnmarshalAnnouncement(t *testing.T) {
	pkr, _, _ := eddilithium2.GenerateKey(nil)
	pk1, sk1, _ := eddilithium2.GenerateKey(nil)
	pk2, sk2, _ := eddilithium2.GenerateKey(nil)
	pk3, sk3, _ := eddilithium2.GenerateKey(nil)
	input := &SwitchAnnouncement{
		Root: Root{
			RootSequence: 1,
		},
	}
	copy(input.RootPublicKey[:], pkr.Bytes())
	var err error
	err = input.Sign(*sk1, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = input.Sign(*sk2, 2)
	if err != nil {
		t.Fatal(err)
	}
	err = input.Sign(*sk3, 3)
	if err != nil {
		t.Fatal(err)
	}
	var buffer [65535]byte
	n, err := input.MarshalBinary(buffer[:])
	if err != nil {
		t.Fatal(err)
	}
	var output SwitchAnnouncement
	if _, err = output.UnmarshalBinary(buffer[:n]); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pkr.Bytes(), output.RootPublicKey[:]) {
		fmt.Println("expected:", pkr)
		fmt.Println("got:", output.RootPublicKey)
		t.Fatalf("first public key doesn't match")
	}
	if len(output.Signatures) < 3 {
		t.Fatalf("not enough signatures were found (should be 3)")
	}
	if !bytes.Equal(pk1.Bytes(), output.Signatures[0].PublicKey[:]) {
		fmt.Println("expected:", pk1)
		fmt.Println("got:", output.Signatures[0].PublicKey)
		t.Fatalf("first public key doesn't match")
	}
	if !bytes.Equal(pk2.Bytes(), output.Signatures[1].PublicKey[:]) {
		fmt.Println("expected:", pk2)
		fmt.Println("got:", output.Signatures[1].PublicKey)
		t.Fatalf("second public key doesn't match")
	}
	if !bytes.Equal(pk3.Bytes(), output.Signatures[2].PublicKey[:]) {
		fmt.Println("expected:", pk3)
		fmt.Println("got:", output.Signatures[2].PublicKey)
		t.Fatalf("third public key doesn't match")
	}
}
