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
	"reflect"
	"testing"
)

func TestDHTRequest(t *testing.T) {
	pk, _, _ := ed25519.GenerateKey(nil)
	req := DHTQueryRequest{
		RequestID: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	copy(req.PublicKey[:], pk)
	var buf [65535]byte
	n, err := req.MarshalBinary(buf[:])
	if err != nil {
		t.Fatal(err)
	}
	orig := DHTQueryRequest{}
	if _, err := orig.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req, orig) {
		t.Fatal("marshalled and unmarshalled structs don't match")
	}
}

func TestDHTResponse(t *testing.T) {
	pk1, sk1, _ := ed25519.GenerateKey(nil)
	pk2, _, _ := ed25519.GenerateKey(nil)
	req := DHTQueryResponse{
		RequestID: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		Results: []DHTNode{
			{
				Coordinates: SwitchPorts{4, 3, 2, 1},
			},
		},
	}
	copy(req.PublicKey[:], pk1)
	copy(req.Results[0].PublicKey[:], pk2)
	var buf [65535]byte
	n, err := req.MarshalBinary(buf[:], sk1)
	if err != nil {
		t.Fatal(err)
	}
	orig := DHTQueryResponse{}
	if _, err := orig.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req, orig) {
		t.Fatal("marshalled and unmarshalled structs don't match")
	}
}
