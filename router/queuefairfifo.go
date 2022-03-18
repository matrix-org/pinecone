// Copyright 2022 The Matrix.org Foundation C.I.C.
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

const fairFIFOQueueCount = 32
const fairFIFOQueueSize = trafficBuffer

type fairFIFOQueue struct {
	queues map[int]chan *types.Frame // queue ID -> frame, map for randomness
	count  int                       // how many queued items in total?
	mutex  sync.Mutex
}

func newFairQueue(_ int) *fairFIFOQueue {
	q := &fairFIFOQueue{}
	q.reset()
	return q
}

func (q *fairFIFOQueue) queuecount() int { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return 0
}

func (q *fairFIFOQueue) queuesize() int { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return fairFIFOQueueCount * trafficBuffer
}

func (q *fairFIFOQueue) hash(frame *types.Frame) int {
	var h uint64
	for _, v := range frame.SourceKey {
		h += uint64(v)
	}
	for _, v := range frame.DestinationKey {
		h += uint64(v)
	}
	return int(h) % fairFIFOQueueCount
}

func (q *fairFIFOQueue) push(frame *types.Frame) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	var h int
	if q.count > 0 {
		h = q.hash(frame) + 1
	}
	select {
	case q.queues[h] <- frame:
		q.count++
		return true
	default:
		return false
	}
}

func (q *fairFIFOQueue) reset() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.queues = make(map[int]chan *types.Frame, fairFIFOQueueCount+1)
	for i := 0; i <= fairFIFOQueueCount; i++ {
		q.queues[i] = make(chan *types.Frame, fairFIFOQueueSize)
	}
}

func (q *fairFIFOQueue) pop() <-chan *types.Frame {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.count == 0 || len(q.queues[0]) > 0 {
		return q.queues[0]
	}
	for _, queue := range q.queues {
		if len(queue) > 0 {
			return queue
		}
	}
	return q.queues[0]
}

func (q *fairFIFOQueue) ack() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.count--
}
