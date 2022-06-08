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

package router

import (
	"sync"

	"github.com/matrix-org/pinecone/types"
)

var frameBufferPool = &sync.Pool{
	New: func() interface{} {
		b := [types.MaxFrameSize]byte{}
		return &b
	},
}

var framePool = &sync.Pool{
	New: func() interface{} {
		f := &types.Frame{
			Payload: make([]byte, 0, types.MaxPayloadSize),
		}
		return f
	},
}

func getFrame() *types.Frame {
	f := framePool.Get().(*types.Frame)
	if f.Refs.Inc() > 1 {
		panic("frame retrieved from pool has unexpected references")
	}
	f.Reset()
	return f
}

func putFrame(f *types.Frame, info ...string) {
	if f.Refs.Dec() > 0 {
		panic("frame still has unexpected references after returning to pool")
	}
	framePool.Put(f)
}
