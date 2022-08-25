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
	"encoding/json"
	"sync"

	"github.com/matrix-org/pinecone/types"
)

type fifoQueue struct {
	log     types.Logger
	max     int
	entries []chan *types.Frame
	mutex   sync.Mutex
}

const fifoNoMax = 0

func newFIFOQueue(max int, log types.Logger) *fifoQueue {
	q := &fifoQueue{
		log: log,
		max: max,
	}
	q.reset()
	return q
}

func (q *fifoQueue) _initialise() {
	for i := range q.entries {
		q.entries[i] = nil
	}
	if q.max != 0 {
		// Make space for one extra entry in the capacity, since
		// every push appends a new channel. To prevent reallocating
		// the whole slice when we hit q.max to increase capacity,
		// make sure there's room for that trailing entry.
		q.entries = make([]chan *types.Frame, 1, q.max+1)
		q.entries[0] = make(chan *types.Frame, 1)
	} else {
		q.entries = []chan *types.Frame{
			make(chan *types.Frame, 1),
		}
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
	if q.max != 0 && len(q.entries)-1 >= q.max {
		return false
	}
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
		case frame := <-ch:
			if frame != nil {
				framePool.Put(frame)
			}
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
	if q.max == 0 && len(q.entries) == 0 {
		q._initialise()
	}
}

func (q *fifoQueue) MarshalJSON() ([]byte, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return json.Marshal(struct {
		Count int `json:"count"`
		Size  int `json:"size"`
	}{
		Count: len(q.entries) - 1,
		Size:  cap(q.entries),
	})
}
