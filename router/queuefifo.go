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

type fifoQueue struct {
	entries []chan *types.Frame
	mutex   sync.Mutex
}

func newFIFOQueue() *fifoQueue {
	q := &fifoQueue{}
	q.reset()
	return q
}

func (q *fifoQueue) _initialise() {
	for i := range q.entries {
		q.entries[i] = nil
	}
	q.entries = []chan *types.Frame{
		make(chan *types.Frame, 1),
	}
}

func (q *fifoQueue) queuecount() int { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return len(q.entries) - 1
}

func (q *fifoQueue) queuesize() int { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return cap(q.entries)
}

func (q *fifoQueue) push(frame *types.Frame) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	ch := q.entries[len(q.entries)-1]
	ch <- frame
	close(ch)
	q.entries = append(q.entries, make(chan *types.Frame, 1))
	return true
}

func (q *fifoQueue) reset() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for _, ch := range q.entries {
		select {
		case <-ch:
		default:
		}
	}
	q._initialise()
}

func (q *fifoQueue) pop() <-chan *types.Frame {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.entries[0]
}

func (q *fifoQueue) ack() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.entries = q.entries[1:]
	if len(q.entries) == 0 {
		q._initialise()
	}
}
