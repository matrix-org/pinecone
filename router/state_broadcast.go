// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/matrix-org/pinecone/router/events"
	"github.com/matrix-org/pinecone/types"
)

type broadcastEntry struct {
	Sequence types.Varu64
	LastSeen time.Time
}

// valid returns true if the broadcast hasn't expired, or false if it has. It is
// required for broadcasts to time out eventually, in the case that nodes leave
// the network and return later.
func (e *broadcastEntry) valid() bool {
	return time.Since(e.LastSeen) < broadcastExpiryPeriod
}

// NOTE: Functions prefixed with an underscore (_) are only safe to be called
// from the actor that owns them, in order to prevent data races.

// _maintainBroadcasts sends out wakeup broadcasts to let local nodes know
// of our presence in the network.
func (s *state) _maintainBroadcasts() {
	select {
	case <-s.r.context.Done():
		return
	default:
		defer s._sendBroadcastIn(wakeupBroadcastInterval)
	}

	// Clean up any broadcasts that are older than the expiry period.
	for k, v := range s._seenBroadcasts {
		if !v.valid() {
			delete(s._seenBroadcasts, k)
		}
	}

	s._sendWakeupBroadcasts()
}

func (s *state) _createBroadcastFrame() (*types.Frame, error) {
	// Construct the broadcast packet.
	b := frameBufferPool.Get().(*[types.MaxFrameSize]byte)
	defer frameBufferPool.Put(b)
	broadcast := types.WakeupBroadcast{
		Sequence: types.Varu64(time.Now().UnixMilli()),
		Root:     s._rootAnnouncement().Root,
	}
	if s.r.secure {
		protected, err := broadcast.ProtectedPayload()
		if err != nil {
			return nil, err
		}
		copy(
			broadcast.Signature[:],
			ed25519.Sign(s.r.private[:], protected),
		)
	}
	n, err := broadcast.MarshalBinary(b[:])
	if err != nil {
		return nil, err
	}

	// Construct the frame.
	send := getFrame()
	send.Type = types.TypeWakeupBroadcast
	send.SourceKey = s.r.public
	send.HopLimit = types.NetworkHorizonDistance
	send.Payload = append(send.Payload[:0], b[:n]...)

	return send, nil
}

func (s *state) _sendWakeupBroadcasts() {
	broadcast, err := s._createBroadcastFrame()
	if err != nil {
		s.r.log.Println("Failed creating broadcast frame:", err)
	}

	s._flood(s.r.local, broadcast, ClassicFlood)
}

func (s *state) _handleBroadcast(p *peer, f *types.Frame) error {
	// Unmarshall the broadcast
	var broadcast types.WakeupBroadcast
	if _, err := broadcast.UnmarshalBinary(f.Payload); err != nil {
		return fmt.Errorf("broadcast unmarshal failed: %w", err)
	}

	if s.r.secure {
		// Check that the broadcast message was protected by the node that claims
		// to have sent it. Silently drop it if there's a signature problem.
		protected, err := broadcast.ProtectedPayload()
		if err != nil {
			return fmt.Errorf("broadcast payload invalid: %w", err)
		}
		if !ed25519.Verify(
			f.SourceKey[:],
			protected,
			broadcast.Signature[:],
		) {
			return fmt.Errorf("broadcast payload signature invalid")
		}
	}

	// Check that the root key in the update matches our current root.
	// If they don't match, silently drop the broadcast.
	root := s._rootAnnouncement()
	if root.RootPublicKey.CompareTo(broadcast.RootPublicKey) != 0 {
		return nil
	}

	// Check the sequence number in the broadcast.
	// If we have seen a higher sequence number before then there is no need
	// to continue forwarding it.
	if existing, ok := s._seenBroadcasts[f.SourceKey]; ok {
		sendingTooFast := time.Since(existing.LastSeen) < broadcastFilterTime
		repeatedSequence := broadcast.Sequence <= existing.Sequence
		if sendingTooFast || repeatedSequence {
			return nil
		}
	}
	s._seenBroadcasts[f.SourceKey] = broadcastEntry{
		Sequence: broadcast.Sequence,
		LastSeen: time.Now(),
	}

	// send event to subscribers about discovered node
	s.r.Act(nil, func() {
		s.r._publish(events.BroadcastReceived{PeerID: f.SourceKey.String(), Time: uint64(time.Now().UnixNano())})
	})

	if f.HopLimit > 1 {
		f.HopLimit -= 1
	} else {
		// The packet has reached the hop limit and shouldn't be forwarded.
		return nil
	}

	// Forward the broadcast to all our peers except for the peer we
	// received it from.
	if f.HopLimit >= types.NetworkHorizonDistance-1 {
		s._flood(p, f, ClassicFlood)
	} else {
		s._flood(p, f, TreeFlood)
	}

	return nil
}
