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
	if len(q.entries) == 0 {
		q._initialise()
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
		case <-ch:
		default:
		}
	}
	q._initialise()
}

func (q *fifoQueue) pop() <-chan *types.Frame {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if len(q.entries) == 0 {
		q._initialise()
	}
	entry := q.entries[0]
	return entry
}

func (q *fifoQueue) ack() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.entries = q.entries[1:]
}
