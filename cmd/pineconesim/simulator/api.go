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

type APIEventMessageID int
type APICommandMessageID int
type APIUpdateID int
type APICommandID int
type APINodeType int

const (
	UnknownEventMsg APIEventMessageID = iota
	SimInitialState
	SimStateUpdate
)

const (
	UnknownCommandMsg APICommandMessageID = iota
	SimPlaySequence
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

const (
	UnknownCommand APICommandID = iota
	SimDebug
	SimPlay
	SimPause
	SimDelay
	SimAddNode
	SimRemoveNode
	SimAddPeer
	SimRemovePeer
	SimConfigureAdversaryDefaults
	SimConfigureAdversaryPeer
	SimStartPings
	SimStopPings
)

const (
	UnknownType APINodeType = iota
	DefaultNode
	GeneralAdversaryNode
)

type InitialNodeState struct {
	PublicKey     string
	NodeType      APINodeType
	RootState     RootState
	Peers         []PeerInfo
	TreeParent    string
	SnakeAsc      string
	SnakeAscPath  string
	SnakeDesc     string
	SnakeDescPath string
}

type RootState struct {
	Root        string
	AnnSequence uint64
	AnnTime     uint64
	Coords      []uint64
}

type PeerInfo struct {
	ID   string
	Port int
}

type SimEventMsg struct {
	UpdateID APIUpdateID
	Event    SimEvent
}

type InitialStateMsg struct {
	MsgID APIEventMessageID
	Nodes map[string]InitialNodeState
	End   bool
}

type StateUpdateMsg struct {
	MsgID APIEventMessageID
	Event SimEventMsg
}

type SimCommandSequenceMsg struct {
	MsgID  APICommandMessageID
	Events []SimCommandMsg
}

type SimCommandMsg struct {
	MsgID APICommandID
	Event interface{}
}
