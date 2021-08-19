package router

import "github.com/matrix-org/pinecone/types"

type queue interface {
	queuecount() int
	queuesize() int
	push(frame *types.Frame) bool
	pop() (*types.Frame, bool)
	reset()
	wait() <-chan struct{}
}
