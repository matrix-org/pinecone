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
	"sort"

	"github.com/matrix-org/pinecone/types"
)

type Overlay struct {
	target types.PublicKey
	ourkey types.PublicKey
	keys   []types.PublicKey
}

func (o *Overlay) candidates() ([]types.PublicKey, error) {
	sort.SliceStable(o.keys, ForwardOrdering(o.ourkey, o.keys))

	mustWrap := o.target.CompareTo(o.ourkey) < 0
	hasWrapped := !mustWrap

	cap, last := len(o.keys), o.keys[0]
	for i, k := range o.keys {
		if hasWrapped {
			if k.CompareTo(o.target) > 0 {
				cap = i
				break
			}
		} else {
			hasWrapped = k.CompareTo(last) < 0
		}
	}
	o.keys = o.keys[:cap]

	nc := len(o.keys)
	if nc > 3 {
		nc = 3
	}

	candidates := []types.PublicKey{}
	candidates = append(candidates, o.keys[:nc]...)

	kc := len(o.keys)
	if kc > 10 {
		candidates = append(candidates, o.keys[kc/8])
	}
	if kc > 7 {
		candidates = append(candidates, o.keys[kc/4])
	}
	if kc > 4 {
		candidates = append(candidates, o.keys[kc/2])
	}

	return candidates, nil
}
