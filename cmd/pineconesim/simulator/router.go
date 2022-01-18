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
	"net"
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
	Ping(ctx context.Context, a net.Addr) (uint16, time.Duration, error)
	Coords() types.Coordinates
	ConfigureFilterDefaults(rates adversary.DropRates)
	ConfigureFilterPeer(peer types.PublicKey, rates adversary.DropRates)
}

type DefaultRouter struct {
	rtr *router.Router
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

func (r *DefaultRouter) Ping(ctx context.Context, a net.Addr) (uint16, time.Duration, error) {
	return r.rtr.Ping(ctx, a)
}

func (r *DefaultRouter) Coords() types.Coordinates {
	return r.rtr.Coords()
}

func (r *DefaultRouter) ConfigureFilterDefaults(rates adversary.DropRates) {}

func (r *DefaultRouter) ConfigureFilterPeer(peer types.PublicKey, rates adversary.DropRates) {}
