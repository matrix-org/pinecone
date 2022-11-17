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
	"math"
	"time"
)

const portCount = math.MaxUint8 - 1
const trafficBuffer = math.MaxUint8 - 1

// peerKeepaliveInterval is the frequency at which this
// node will send keepalive packets to other peers if no
// other packets have been sent within the peerKeepaliveInterval.
const peerKeepaliveInterval = time.Second * 3

// peerKeepaliveTimeout is the amount of time that must
// pass without receiving any packet before we
// will assume that the peer is dead.
const peerKeepaliveTimeout = time.Second * 5

// announcementInterval is the frequency at which this
// node will send root announcements to other peers.
const announcementInterval = time.Minute * 30

// announcementTimeout is the amount of time that must
// pass without receiving a root announcement before we
// will assume that the peer is dead.
const announcementTimeout = time.Minute * 45

// virtualSnakeMaintainInterval is how often we check to
// see if SNEK maintenance needs to be done.
const virtualSnakeMaintainInterval = time.Second

// virtualSnakeBootstrapInterval is how often we will aim
// to send bootstrap messages into the network.
const virtualSnakeBootstrapInterval = time.Second * 5

// virtualSnakeNeighExpiryPeriod is how long we'll wait
// to expire a path that hasn't re-bootstrapped.
const virtualSnakeNeighExpiryPeriod = virtualSnakeBootstrapInterval * 2

// coordsCacheLifetime is how long we'll keep entries in
// the coords cache for switching to tree routing.
const coordsCacheLifetime = time.Minute

// coordsCacheMaintainInterval is how often we will clean
// out stale entries from the coords cache.
const coordsCacheMaintainInterval = time.Minute

// wakeupBroadcastInterval is how often we will aim
// to send broadcast messages into the network.
const wakeupBroadcastInterval = time.Minute

// broadcastExpiryPeriod is how long we'll wait to
// expire a seen broadcast.
const broadcastExpiryPeriod = wakeupBroadcastInterval * 3

// broadcastFilterTime is how much time must pass
// before we'll accept a new broadcast from this node.
// This helps to prevent broadcasts from flooding the
// network.
const broadcastFilterTime = wakeupBroadcastInterval / 2
