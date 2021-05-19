package router

import (
	"sync"

	"github.com/matrix-org/pinecone/types"
)

type queue struct {
	frames []*types.Frame
	size   int
	count  int
	mutex  sync.Mutex
	notifs chan struct{}
}

func newQueue(size int) *queue {
	q := &queue{
		frames: make([]*types.Frame, size),
		size:   size,
		notifs: make(chan struct{}, size),
	}
	return q
}

func (q *queue) push(frame *types.Frame) bool {
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

func (q *queue) pop() (*types.Frame, bool) {
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

func (q *queue) reset() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.count = 0
	for i := range q.frames {
		q.frames[i] = nil
	}
	for range q.notifs {
	}
}

func (q *queue) wait() <-chan struct{} {
	return q.notifs
}
