package router

import (
	"testing"

	"github.com/matrix-org/pinecone/types"
)

func TestFIFOQueue(t *testing.T) {
	q := newFIFOQueue()
	iterations := types.SwitchPortID(1024)

	go func() {
		for i := types.SwitchPortID(0); i < iterations; i++ {
			q.push(&types.Frame{
				Destination: types.SwitchPorts{i},
			})
		}
	}()

	got := types.SwitchPortID(0)

	for i := types.SwitchPortID(0); i < iterations; i++ {
		frame := <-q.pop()
		if frame == nil {
			t.Fatalf("unexpected nil frame")
		}
		if frame.Destination[0] < got || frame.Destination[0] > got+1 {
			t.Fatalf("ordering problem")
		}
		got = frame.Destination[0]
		q.ack()
	}
}
