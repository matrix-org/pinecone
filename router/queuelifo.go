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

type lifoQueue struct { // nolint:unused
	frames []*types.Frame
	size   int
	count  int
	mutex  sync.Mutex
	notifs chan struct{}
}

func newLIFOQueue(size int) *lifoQueue { // nolint:unused,deadcode
	q := &lifoQueue{
		frames: make([]*types.Frame, size),
		size:   size,
		notifs: make(chan struct{}, size),
	}
	return q
}

func (q *lifoQueue) queuecount() int { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.count
}

func (q *lifoQueue) queuesize() int { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.size
}

func (q *lifoQueue) push(frame *types.Frame) bool { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.count == q.size {
		return false
	}
	index := q.size - q.count - 1
	q.frames[index] = frame
	q.count++
	select {
	case q.notifs <- struct{}{}:
	default:
		panic("this should be impossible")
	}
	return true
}

func (q *lifoQueue) pop() (*types.Frame, bool) { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.count == 0 {
		return nil, false
	}
	index := q.size - q.count
	frame := q.frames[index]
	q.frames[index] = nil
	q.count--
	return frame, true
}

func (q *lifoQueue) ack() { // nolint:unused
	// no-op on this queue type
}

func (q *lifoQueue) reset() { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.count = 0
	for i := range q.frames {
		if q.frames[i] != nil {
			framePool.Put(q.frames[i])
			q.frames[i] = nil
		}
	}
	close(q.notifs)
	for range q.notifs {
	}
	q.notifs = make(chan struct{}, q.size)
}

func (q *lifoQueue) wait() <-chan struct{} { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.notifs
}
