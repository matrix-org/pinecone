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

type APIMessageID int
type APIUpdateID int

const (
	UnknownMessage APIMessageID = iota
	SimInitialState
	SimUpdate
)

const (
	UnknownUpdate APIUpdateID = iota
	SimNodeAdded
	SimNodeRemoved
	SimPeerAdded
	SimPeerRemoved
	SimTreeParentUpdated
	SimSnakeAscUpdated
	SimSnakeDescUpdated
	SimTreeRootAnnUpdated
)

type RootState struct {
	Root        string
	AnnSequence uint64
	AnnTime     uint64
	Coords      []uint64
}

type SimEventMsg struct {
	UpdateID APIUpdateID
	Event    SimEvent
}

type InitialStateMsg struct {
	MsgID      APIMessageID
	Nodes      []string
	RootState  map[string]RootState
	PeerEdges  map[string][]string
	SnakeEdges map[string][]string
	TreeEdges  map[string]string
	End        bool
}

type StateUpdateMsg struct {
	MsgID APIMessageID
	Event SimEventMsg
}
