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
	"fmt"
	"runtime/debug"
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

var lastFrame *types.Frame

func getFrame() *types.Frame {
	f := &types.Frame{
		Payload: make([]byte, 0, types.MaxPayloadSize),
	}
	if f == lastFrame {
		panic("allocated same memory")
	}
	lastFrame = f
	return f
	/*f := framePool.Get().(*types.Frame)
	f.Returns = f.Returns[:0]
	f.Reset()
	return f
	*/
}

func putFrame(f *types.Frame, info ...string) {
	f.Lock()
	defer f.Unlock()
	f.Reset()
	var trace []byte
	for _, i := range info {
		trace = append(trace, []byte(i)...)
		trace = append(trace, ' ')
	}
	trace = append(trace, debug.Stack()...)
	f.Returns = append(f.Returns, trace)
	if len(f.Returns) > 1 {
		for _, r := range f.Returns {
			fmt.Println("***", string(r))
		}
		panic(fmt.Sprintf("multiple putFrame calls for %p type %q", f, f.Type))
	}
	//framePool.Put(f)
}
