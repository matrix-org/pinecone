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

package events

import (
	"github.com/matrix-org/pinecone/types"
)

type Event interface {
	isEvent()
}

type PeerAdded struct {
	Port   types.SwitchPortID
	PeerID string
}

// Tag PeerAdded as an Event
func (e PeerAdded) isEvent() {}

type PeerRemoved struct {
	Port   types.SwitchPortID
	PeerID string
}

// Tag PeerRemoved as an Event
func (e PeerRemoved) isEvent() {}

type TreeParentUpdate struct {
	PeerID string
}

// Tag TreeParentUpdate as an Event
func (e TreeParentUpdate) isEvent() {}

type SnakeDescUpdate struct {
	PeerID string
}

// Tag SnakeDescUpdate as an Event
func (e SnakeDescUpdate) isEvent() {}

type TreeRootAnnUpdate struct {
	Root     string // Root Public Key
	Sequence uint64
	Time     uint64 // Unix Time
	Coords   []uint64
}

// Tag TreeRootAnnUpdate as an Event
func (e TreeRootAnnUpdate) isEvent() {}

type SnakeEntryAdded struct {
	EntryID string
	PeerID  string
}

// Tag SnakeEntryAdded as an Event
func (e SnakeEntryAdded) isEvent() {}

type SnakeEntryRemoved struct {
	EntryID string
}

// Tag SnakeEntryRemoved as an Event
func (e SnakeEntryRemoved) isEvent() {}

type BroadcastReceived struct {
	PeerID string
	Time   uint64
}

// Tag BroadcastReceived as an Event
func (e BroadcastReceived) isEvent() {}

type PeerBandwidthUsage struct {
	Protocol struct {
		Rx uint64
		Tx uint64
	}
	Overlay struct {
		Rx uint64
		Tx uint64
	}
}

type BandwidthReport struct {
	CaptureTime uint64 // Unix Time
	Peers       map[string]PeerBandwidthUsage
}

// Tag BandwidthReport as an Event
func (e BandwidthReport) isEvent() {}
