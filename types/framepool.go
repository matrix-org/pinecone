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
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
)

var framePool = &sync.Pool{
	New: func() interface{} {
		f := &Frame{
			Payload: make([]byte, 0, MaxFrameSize),
		}
		runtime.SetFinalizer(f, func(f *Frame) {
			if refs := f.refs.Load(); refs != 0 {
				fmt.Println("Frame type:", f.Type)
				fmt.Println("Frame taken out at:")
				fmt.Println(string(f.stack))
				panic(fmt.Sprintf("frame was garbage collected with %d remaining references, this is a bug", refs))
			}
		})
		return f
	},
}

func GetFrame() *Frame {
	frame := framePool.Get().(*Frame)
	if !frame.refs.CAS(0, 1) {
		panic("invalid frame reuse")
	}
	frame.Reset()
	frame.stack = debug.Stack()
	return frame
}

func (f *Frame) Reset() {
	f.Version, f.Type = 0, 0
	f.Destination = SwitchPorts{}
	f.DestinationKey = PublicKey{}
	f.Source = SwitchPorts{}
	f.SourceKey = PublicKey{}
	f.Payload = f.Payload[:0]
}

func (f *Frame) Copy() *Frame {
	copy := GetFrame()
	copy.Version, copy.Type = f.Version, f.Type
	copy.Destination = f.Destination
	copy.DestinationKey = f.DestinationKey
	copy.Source = f.Source
	copy.SourceKey = f.SourceKey
	copy.Payload = append(copy.Payload[:0], f.Payload...)
	return copy
}

func (f *Frame) Borrow() *Frame {
	if f.refs.Load() == 0 {
		panic("invalid Borrow call after final Done call")
	}
	f.refs.Inc()
	return f
}

func (f *Frame) Done() bool {
	refs := f.refs.Dec()
	if refs < 0 {
		panic("invalid Done call")
	}
	if refs == 0 {
		f.Reset()
		framePool.Put(f)
		return true
	}
	return false
}
