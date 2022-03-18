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
	queues map[uint8]chan *types.Frame // queue ID -> frame, map for randomness
	count  int                         // how many queued items in total?
	mutex  sync.Mutex
}

func newFairFIFOQueue(_ int) *fairFIFOQueue {
	q := &fairFIFOQueue{}
	q.reset()
	return q
}

func (q *fairFIFOQueue) queuecount() int { // nolint:unused
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.count
}

func (q *fairFIFOQueue) queuesize() int { // nolint:unused
	return fairFIFOQueueCount * fairFIFOQueueSize
}

func (q *fairFIFOQueue) hash(frame *types.Frame) uint8 {
	var h uint64
	for _, v := range frame.SourceKey {
		h += uint64(v)
	}
	return uint8(h) % fairFIFOQueueCount
}

func (q *fairFIFOQueue) push(frame *types.Frame) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	var h uint8
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
	q.queues = make(map[uint8]chan *types.Frame, fairFIFOQueueCount+1)
	for i := uint8(0); i <= fairFIFOQueueCount; i++ {
		q.queues[i] = make(chan *types.Frame, fairFIFOQueueSize)
	}
}

func (q *fairFIFOQueue) pop() <-chan *types.Frame {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	switch {
	case q.count == 0:
		// Nothing has been queued yet — whatever gets queued up
		// next will always be in queue 0, so it makes sense to
		// return the channel for that queue for now.
		fallthrough
	case len(q.queues[0]) > 0:
		// There's something in queue 0 waiting to be sent.
		return q.queues[0]
	default:
		// Select a random queue that has something waiting.
		for i, queue := range q.queues {
			if i == 0 {
				continue
			}
			if len(queue) > 0 {
				return queue
			}
		}
	}
	// We shouldn't ever arrive here.
	panic("invalid queue state")
}

func (q *fairFIFOQueue) ack() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.count--
}
