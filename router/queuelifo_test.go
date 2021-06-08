package router

import (
	"testing"

	"github.com/matrix-org/pinecone/types"
)

func TestQueue(t *testing.T) {
	one := &types.Frame{}
	two := &types.Frame{}
	three := &types.Frame{}
	four := &types.Frame{}
	queue := newLIFOQueue(3)
	if !queue.push(one) {
		t.Fatalf("first frame should have pushed")
	}
	if !queue.push(two) {
		t.Fatalf("second frame should have pushed")
	}
	if !queue.push(three) {
		t.Fatalf("third frame should have pushed")
	}
	if queue.push(four) {
		t.Fatalf("fourth frame should not have pushed")
	}
	if frame, ok := queue.pop(); !ok || frame != three {
		t.Fatalf("first pop should have been third frame")
	}
	if frame, ok := queue.pop(); !ok || frame != two {
		t.Fatalf("second pop should have been second frame")
	}
	if frame, ok := queue.pop(); !ok || frame != one {
		t.Fatalf("third pop should have been first frame")
	}
	if _, ok := queue.pop(); ok {
		t.Fatalf("fourth pop have returned nothing")
	}
}
