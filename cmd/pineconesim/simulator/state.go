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
	"fmt"

	"github.com/Arceliar/phony"
)

type RootAnnouncement struct {
	Root     string
	Sequence uint64
	Time     uint64
}

type NodeState struct {
	PeerID         string
	Connections    map[int]string
	Parent         string
	Coords         []uint64
	Announcement   RootAnnouncement
	AscendingPeer  string
	DescendingPeer string
}

func NewNodeState(peerID string) *NodeState {
	node := &NodeState{
		PeerID:         peerID,
		Connections:    make(map[int]string),
		Parent:         "",
		Announcement:   RootAnnouncement{},
		Coords:         []uint64{},
		AscendingPeer:  "",
		DescendingPeer: "",
	}
	return node
}

type State struct {
	Nodes map[string]*NodeState
}

func NewState() *State {
	state := &State{
		Nodes: make(map[string]*NodeState),
	}
	return state
}

type StateAccessor struct {
	phony.Inbox
	_subscribers map[chan<- SimEvent]*phony.Inbox
	_state       *State
}

func NewStateAccessor() *StateAccessor {
	sa := &StateAccessor{
		_state:       NewState(),
		_subscribers: make(map[chan<- SimEvent]*phony.Inbox),
	}
	return sa
}

func (s *StateAccessor) Subscribe(ch chan<- SimEvent) State {
	var stateCopy State
	phony.Block(s, func() {
		s._subscribers[ch] = &phony.Inbox{}
		stateCopy = *s._state
	})
	return stateCopy
}

func (s *StateAccessor) GetLinkCount() float64 {
	count := 0.0
	phony.Block(s, func() {
		for _, node := range s._state.Nodes {
			for range node.Connections {
				// Each peer connection represents half of a physical link between nodes
				count += 0.5
			}
		}
	})
	return count
}

func (s *StateAccessor) GetNodeName(peerID string) (string, error) {
	node := ""
	err := fmt.Errorf("Provided peerID is not associated with a known node")

	phony.Block(s, func() {
		for k, v := range s._state.Nodes {
			if v.PeerID == peerID {
				node, err = k, nil
			}
		}
	})
	return node, err
}

func (s *StateAccessor) _addNode(name string, peerID string) {
	s._state.Nodes[name] = NewNodeState(peerID)
	s._publish(NodeAdded{Node: name})
}

func (s *StateAccessor) _addPeerConnection(from string, to string, port int) {
	if _, ok := s._state.Nodes[from]; ok {
		s._state.Nodes[from].Connections[port] = to
	}
	s._publish(PeerAdded{Node: from, Peer: to})
}

func (s *StateAccessor) _removePeerConnection(from string, to string, port int) {
	if _, ok := s._state.Nodes[from]; ok {
		delete(s._state.Nodes[from].Connections, port)
	}
	s._publish(PeerRemoved{Node: from, Peer: to})
}

func (s *StateAccessor) _updateParent(node string, peerID string) {
	if _, ok := s._state.Nodes[node]; ok {
		prev := s._state.Nodes[node].Parent
		s._state.Nodes[node].Parent = peerID

		s._publish(TreeParentUpdate{Node: node, Peer: peerID, Prev: prev})
	}
}

func (s *StateAccessor) _updateAscendingPeer(node string, peerID string) {
	if _, ok := s._state.Nodes[node]; ok {
		prev := s._state.Nodes[node].AscendingPeer
		s._state.Nodes[node].AscendingPeer = peerID

		s._publish(SnakeAscUpdate{Node: node, Peer: peerID, Prev: prev})
	}
}

func (s *StateAccessor) _updateDescendingPeer(node string, peerID string) {
	if _, ok := s._state.Nodes[node]; ok {
		prev := s._state.Nodes[node].DescendingPeer
		s._state.Nodes[node].DescendingPeer = peerID

		s._publish(SnakeDescUpdate{Node: node, Peer: peerID, Prev: prev})
	}
}

func (s *StateAccessor) _updateTreeRootAnnouncement(node string, root string, sequence uint64, time uint64, coords []uint64) {
	if _, ok := s._state.Nodes[node]; ok {
		s._state.Nodes[node].Announcement.Root = root
		s._state.Nodes[node].Announcement.Sequence = sequence
		s._state.Nodes[node].Announcement.Time = time
		s._state.Nodes[node].Coords = coords

		s._publish(TreeRootAnnUpdate{
			Node:     node,
			Root:     root,
			Sequence: sequence,
			Time:     time,
			Coords:   coords})
	}
}

func (s *StateAccessor) _publish(event SimEvent) {
	for ch, inbox := range s._subscribers {
		inbox.Act(nil, func() {
			ch <- event
		})
	}
}
