package router

import (
	"sync"

	"github.com/matrix-org/pinecone/types"
)

type fifoQueue struct {
	frames []*types.Frame
	count  int
	mutex  sync.Mutex
	notifs chan struct{}
}

func newFIFOQueue() *fifoQueue {
	q := &fifoQueue{
		notifs: make(chan struct{}),
	}
	return q
}

func (q *fifoQueue) queuecount() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.count
}

func (q *fifoQueue) queuesize() int {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return cap(q.frames)
}

func (q *fifoQueue) push(frame *types.Frame) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.frames = append(q.frames, frame)
	q.count++
	select {
	case q.notifs <- struct{}{}:
	default:
	}
	return true
}

func (q *fifoQueue) pop() (*types.Frame, bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.count == 0 {
		return nil, false
	}
	frame := q.frames[0]
	q.frames[0] = nil
	q.frames = q.frames[1:]
	q.count--
	if q.count == 0 {
		// Force a GC of the underlying array, since it might have
		// grown significantly if the queue was hammered for some reason
		q.frames = nil
	}
	return frame, true
}

func (q *fifoQueue) reset() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.count = 0
	for i := range q.frames {
		if q.frames[i] != nil {
			q.frames[i].Done()
			q.frames[i] = nil
		}
	}
	q.frames = nil
	close(q.notifs)
	for range q.notifs {
	}
	q.notifs = make(chan struct{})
}

func (q *fifoQueue) wait() <-chan struct{} {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.count > 0 {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return q.notifs
}
