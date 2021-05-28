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

package router

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/matrix-org/pinecone/types"
)

// The pathfinder is used to build source routes from either greedy or
// snake routing. Generally we don't use this for actual packet routing
// today, although the simulator uses it to determine the path length vs
// real path length. An application using Pinecone as a library can use
// the pathfinder to switch from greedy/snake routing to source routing
// if desired.
type pathfinder struct {
	r        *Router
	requests sync.Map
}

// newPathfinder returns a new pathfinding instance. This can be used to
// perform an active pathfind between two nodes using either greedy or
// snake routing and convert it to
func newPathfinder(r *Router) *pathfinder {
	return &pathfinder{
		r: r,
	}
}

func (p *pathfinder) pathfind(ctx context.Context, addr net.Addr) (net.Addr, error) {
	var returnPath types.SwitchPorts
	payload := [2]byte{0, 0}
	searchContext := p.newPathfind(ctx, addr)

	switch a := addr.(type) {
	case GreedyAddr:
		if a.SwitchPorts.EqualTo(p.r.Coords()) {
			return SourceAddr{types.SwitchPorts{}}, nil
		}
		select {
		case p.r.send <- types.Frame{
			Version:     types.Version0,
			Type:        types.TypePathfind,
			Destination: a.SwitchPorts,
			Source:      p.r.Coords(),
			Payload:     payload[:],
		}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case res := <-searchContext.ch:
			if out, in := res.res.ReturnPath(false), res.res.ReturnPath(true); len(in) <= len(out) {
				returnPath = in
			} else {
				returnPath = out
			}
			if p.r.simulator != nil {
				if id, err := p.r.simulator.LookupNodeID(a.SwitchPorts); err == nil {
					p.r.simulator.ReportDistance(p.r.id, id, int64(len(returnPath)))
				}
			}
			return SourceAddr{returnPath}, nil
		case <-searchContext.ctx.Done():
			return nil, searchContext.ctx.Err()
		}

	case types.PublicKey:
		if a.EqualTo(p.r.PublicKey()) {
			return SourceAddr{types.SwitchPorts{}}, nil
		}
		select {
		case p.r.send <- types.Frame{
			Version:        types.Version0,
			Type:           types.TypeVirtualSnakePathfind,
			DestinationKey: a,
			SourceKey:      p.r.PublicKey(),
			Payload:        payload[:],
		}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case res := <-searchContext.ch:
			if out, in := res.res.ReturnPath(false), res.res.ReturnPath(true); len(in) <= len(out) {
				returnPath = in
			} else {
				returnPath = out
			}
			if p.r.simulator != nil {
				if id, err := p.r.simulator.LookupPublicKey(a); err == nil {
					p.r.simulator.ReportDistance(p.r.id, id, int64(len(returnPath)))
				}
			}
			return SourceAddr{returnPath}, nil
		case <-searchContext.ctx.Done():
			return nil, searchContext.ctx.Err()
		}

	default:
		return nil, fmt.Errorf("expected *GreedyAddr or *GreedySNEKAddr as net.Addr")
	}
}

func (r *Router) signPathfind(frame *types.Frame, from, to *Peer) (*types.Frame, error) {
	var pathfind types.Pathfind
	signedFrame := frame.Copy()
	if _, err := pathfind.UnmarshalBinary(frame.Payload); err != nil {
		return nil, fmt.Errorf("pathfind.UnmarshalBinary: %w", err)
	}
	for _, sig := range pathfind.Signatures[pathfind.Boundary:] {
		if r.public.EqualTo(sig.PublicKey) {
			// The pathfind frame looped
			return nil, nil
		}
	}
	port := to.port
	if pathfind.Boundary > 0 {
		port = from.port
	}
	if port == 0 {
		return signedFrame, nil
	}
	signed, err := pathfind.Sign(r.private[:], port)
	if err != nil {
		return nil, fmt.Errorf("pathfind.Sign: %w", err)
	}
	var buf [MaxPayloadSize]byte
	n, err := signed.MarshalBinary(buf[:])
	if err != nil {
		return nil, fmt.Errorf("signed.MarshalBinary: %w", err)
	}
	signedFrame.Payload = buf[:n]
	return signedFrame, nil
}

// pathfindContext represents an ongoing pathfind.
type pathfindContext struct {
	id  net.Addr
	ctx context.Context
	ch  chan *pathfindResponse
}

type pathfindResponse struct {
	from net.Addr
	res  *types.Pathfind
}

// pathfind creates a new pathfindContext and returns it.
func (p *pathfinder) newPathfind(ctx context.Context, addr net.Addr) *pathfindContext {
	// TODO: use something better than a stringified key
	if c, ok := p.requests.Load(addr.String()); ok {
		return c.(*pathfindContext)
	}

	// Build a new timeout and response channel. If we get a
	// path response back, it will be sent into the matching
	// channel.
	ch := make(chan *pathfindResponse, 1)
	req := &pathfindContext{addr, ctx, ch}
	// TODO: It's not nice to generate a map key in this way.
	p.requests.Store(addr.String(), req)
	return req
}

func (p *pathfinder) onPathfindResponse(addr net.Addr, res types.Pathfind) {
	// TODO: use something better than a stringified key
	key := addr.String()
	if req, ok := p.requests.Load(key); ok {
		if dhtReq, ok := req.(*pathfindContext); ok {
			dhtReq.ch <- &pathfindResponse{addr, &res}
			p.requests.Delete(key)
		}
	}
}
