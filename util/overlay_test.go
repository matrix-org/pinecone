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

package util

import (
	"fmt"
	"testing"

	"github.com/cloudflare/circl/sign/eddilithium2"
	"github.com/matrix-org/pinecone/types"
)

func TestOverlaySorting(t *testing.T) {
	overlay := &Overlay{}
	opk, _, _ := eddilithium2.GenerateKey(nil)
	tpk, _, _ := eddilithium2.GenerateKey(nil)
	copy(overlay.ourkey[:], opk.Bytes())
	copy(overlay.target[:], tpk.Bytes())

	fmt.Println("Our key:   ", overlay.ourkey)
	fmt.Println("Target key:", overlay.target)

	for i := 0; i < 32; i++ {
		pk, _, _ := eddilithium2.GenerateKey(nil)
		k := types.PublicKey{}
		copy(k[:], pk.Bytes())
		overlay.keys = append(overlay.keys[:], k)
	}

	fmt.Println("Candidates:")
	candidates, err := overlay.candidates()
	if err != nil {
		panic(err)
	}
	for _, k := range candidates {
		fmt.Println("*", k)
	}
}
