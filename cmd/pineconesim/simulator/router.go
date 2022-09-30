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

package simulator

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator/adversary"
	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/router/events"
	"github.com/matrix-org/pinecone/types"
)

type SimRouter interface {
	PublicKey() types.PublicKey
	Connect(conn net.Conn, options ...router.ConnectionOption) (types.SwitchPortID, error)
	Subscribe(ch chan events.Event)
	Ping(ctx context.Context, a types.PublicKey) (uint16, time.Duration, error)
	Coords() types.Coordinates
	ConfigureFilterDefaults(rates adversary.DropRates)
	ConfigureFilterPeer(peer types.PublicKey, rates adversary.DropRates)
	ManholeHandler(w http.ResponseWriter, req *http.Request)
}

type DefaultRouter struct {
	rtr   *router.Router
	pings sync.Map // types.PublicKey -> chan struct{}
}

func (r *DefaultRouter) Subscribe(ch chan events.Event) {
	r.rtr.Subscribe(ch)
}

func (r *DefaultRouter) PublicKey() types.PublicKey {
	return r.rtr.PublicKey()
}

func (r *DefaultRouter) Connect(conn net.Conn, options ...router.ConnectionOption) (types.SwitchPortID, error) {
	return r.rtr.Connect(conn, options...)
}

func (r *DefaultRouter) Coords() types.Coordinates {
	return r.rtr.Coords()
}

func (r *DefaultRouter) EnableHopLimiting() {
	r.rtr.EnableHopLimiting()
}

func (r *DefaultRouter) EnableWakeupBroadcasts() {
	r.rtr.EnableWakeupBroadcasts()
}

func (r *DefaultRouter) DisableWakeupBroadcasts() {
	r.rtr.DisableWakeupBroadcasts()
}

func (r *DefaultRouter) ConfigureFilterDefaults(rates adversary.DropRates) {}

func (r *DefaultRouter) ConfigureFilterPeer(peer types.PublicKey, rates adversary.DropRates) {}

func (r *DefaultRouter) ManholeHandler(w http.ResponseWriter, req *http.Request) {
	r.rtr.ManholeHandler(w, req)
}

func (r *DefaultRouter) Ping(ctx context.Context, destination types.PublicKey) (uint16, time.Duration, error) {
	id := destination.String()
	payload := PingPayload{
		origin:      r.PublicKey(),
		destination: destination,
		hops:        1,
	}

	p := make([]byte, pingSize)
	_, err := payload.MarshalBinary(p)
	if err != nil {
		return 0, 0, fmt.Errorf("failed marshalling ping payload: %w", err)
	}

	_, writeErr := r.rtr.WriteTo(p, destination)
	if writeErr != nil {
		return 0, 0, fmt.Errorf("failed sending ping to node: %w", writeErr)
	}

	start := time.Now()
	v, existing := r.pings.LoadOrStore(id, make(chan uint16))
	if existing {
		return 0, 0, fmt.Errorf("a ping to this node is already in progress")
	}
	defer r.pings.Delete(id)
	ch := v.(chan uint16)
	select {
	case <-ctx.Done():
		return 0, 0, fmt.Errorf("ping timed out")
	case hops := <-ch:
		return hops, time.Since(start), nil
	}
}

func (r *DefaultRouter) PingFilter(from types.PublicKey, f *types.Frame) bool {
	if !f.Type.IsTraffic() {
		return false
	}

	payload := PingPayload{}
	if _, err := payload.UnmarshalBinary(f.Payload); err != nil {
		return false
	}
	if payload.pingType == Ping {
		payload.hops++
		if _, err := payload.MarshalBinary(f.Payload); err != nil {
			return true
		}
	}

	return false
}

func (r *DefaultRouter) OverlayReadHandler(quit <-chan bool) {
	buf := make([]byte, types.MaxPayloadSize)
	for {
		select {
		case <-quit:
			return
		default:
		}

		if err := r.rtr.SetReadDeadline(time.Now().Add(time.Millisecond * 300)); err != nil {
			panic(err)
		}
		n, _, err := r.rtr.ReadFrom(buf)
		if err != nil || n == 0 {
			continue
		}

		payload := PingPayload{}
		if _, err = payload.UnmarshalBinary(buf[:n]); err != nil {
			continue
		}

		switch payload.pingType {
		case Ping:
			if payload.destination == r.PublicKey() {
				payload.pingType = Pong
			} else {
				println("PING: hit deadend at:", r.PublicKey().String(), "for:", payload.origin.String(), "to:", payload.destination.String())
				continue
			}
		case Pong:
			if payload.origin == r.PublicKey() {
				id := payload.destination.String()
				v, ok := r.pings.Load(id)
				if !ok {
					continue
				}
				ch := v.(chan uint16)
				ch <- payload.hops
				close(ch)
				r.pings.Delete(id)
				continue
			} else {
				println("PONG: hit deadend at:", r.PublicKey().String(), "for:", payload.origin.String(), "to:", payload.destination.String())
				continue
			}
		default:
			continue
		}

		if n, err = payload.MarshalBinary(buf); err != nil {
			continue
		}
		if _, err = r.rtr.WriteTo(buf[:n], payload.origin); err != nil {
			continue
		}
	}
}
