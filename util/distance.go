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
	"crypto/ed25519"

	"github.com/matrix-org/pinecone/types"
)

func LessThan(first, second types.PublicKey) bool {
	for i := 0; i < ed25519.PublicKeySize; i++ {
		if first[i] < second[i] {
			return true
		}
		if first[i] > second[i] {
			return false
		}
	}
	return false
}

// DHTOrdered returns true if the order of A, B and C is
// correct, where A < B < C without wrapping.
func DHTOrdered(a, b, c types.PublicKey) bool {
	return LessThan(a, b) && LessThan(b, c)
}

// DHTWrappedOrdered returns true if the ordering of A, B
// and C is correct, where we may wrap around from C to A.
// This gives us the property of the successor always being
// a+1 and the predecessor being a+sizeofkeyspace.
func DHTWrappedOrdered(a, b, c types.PublicKey) bool {
	ab, bc, ca := LessThan(a, b), LessThan(b, c), LessThan(c, a)
	switch {
	case ab && bc:
		return true
	case bc && ca:
		return true
	case ca && ab:
		return true
	}
	return false
}

func ReverseOrdering(target types.PublicKey, input []types.PublicKey) func(i, j int) bool {
	return func(i, j int) bool {
		return DHTWrappedOrdered(input[i], target, input[j])
	}
}

func ForwardOrdering(target types.PublicKey, input []types.PublicKey) func(i, j int) bool {
	return func(i, j int) bool {
		return DHTWrappedOrdered(target, input[i], input[j])
	}
}
