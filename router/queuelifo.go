package router

import (
	"sync"

	"github.com/matrix-org/pinecone/types"
)

type lifoQueue struct {
	frames []*types.Frame
	size   int
	count  int
	mutex  sync.Mutex
	notifs chan struct{}
}

func newLIFOQueue(size int) *lifoQueue {
	q := &lifoQueue{
		frames: make([]*types.Frame, size),
		size:   size,
		notifs: make(chan struct{}, size),
	}
	return q
}

func (q *lifoQueue) queuecount() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.count
}

func (q *lifoQueue) queuesize() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.size
}

func (q *lifoQueue) push(frame *types.Frame) bool {
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

func (q *lifoQueue) pop() (*types.Frame, bool) {
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

func (q *lifoQueue) reset() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.count = 0
	for i := range q.frames {
		if q.frames[i] != nil {
			q.frames[i].Done()
			q.frames[i] = nil
		}
	}
	close(q.notifs)
	for range q.notifs {
	}
	q.notifs = make(chan struct{}, q.size)
}

func (q *lifoQueue) wait() <-chan struct{} {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.notifs
}
