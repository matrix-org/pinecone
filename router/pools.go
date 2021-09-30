package router

import (
	"sync"

	"github.com/matrix-org/pinecone/types"
)

var frameBufferPool = &sync.Pool{
	New: func() interface{} {
		b := [types.MaxFrameSize]byte{}
		return &b
	},
}

/*
var framePool = &sync.Pool{
	New: func() interface{} {
		f := &types.Frame{
			Payload: make([]byte, 0, types.MaxPayloadSize),
		}
		return f
	},
}
*/
