package router

import (
	"testing"

	"github.com/matrix-org/pinecone/types"
)

func TestLimitedFIFO(t *testing.T) {
	q := newFIFOQueue(5, nil)

	// the actual allocated queue size will be 1 more than the
	// supplied, so that when we push an entry and assign the
	// next channel, that won't cause a reallocation of the queue
	if s := q.queuesize(); s != 6 {
		t.Fatalf("expected queue size to be 6 but it was %d", s)
	}

	for i := 0; i < 10; i++ {
		added := q.push(&types.Frame{})
		switch {
		case i < 5 && !added:
			t.Fatalf("expected %d to be added", i)
		case i >= 5 && added:
			t.Fatalf("expected %d to not be added", i)
		}
	}

	if s := q.queuecount(); s != 5 {
		t.Fatalf("expected final queue count to be 5 but it was %d", s)
	}
	if s := q.queuesize(); s != 6 {
		t.Fatalf("expected final queue size to be 6 but it was %d", s)
	}
}
