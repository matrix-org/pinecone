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
	"sort"
	"strings"
	"testing"

	"github.com/matrix-org/pinecone/types"
)

func TestDHTWrappedOrdering(t *testing.T) {
	target := types.PublicKey{5}
	input := []types.PublicKey{
		{1}, {2}, {3}, {9}, {7}, {4}, {6}, {8}, {0},
	}
	reverse := []types.PublicKey{
		{4}, {3}, {2}, {1}, {0}, {9}, {8}, {7}, {6},
	}
	forward := []types.PublicKey{
		{6}, {7}, {8}, {9}, {0}, {1}, {2}, {3}, {4},
	}

	out := func(show []types.PublicKey) string {
		var parts []string
		for _, i := range show {
			parts = append(parts, fmt.Sprintf("%d", i[0]))
		}
		return strings.Join(parts, ", ")
	}

	sort.SliceStable(input, ForwardOrdering(target, input))
	for i := range input {
		if input[i] != forward[i] {
			t.Log("Want:", out(forward))
			t.Log("Got: ", out(input))
			t.Fatalf("Successor ordering incorrect")
		}
	}

	sort.SliceStable(input, ReverseOrdering(target, input))
	for i := range input {
		if input[i] != reverse[i] {
			t.Log("Want:", out(reverse))
			t.Log("Got: ", out(input))
			t.Fatalf("Predecessor ordering incorrect")
		}
	}
}
