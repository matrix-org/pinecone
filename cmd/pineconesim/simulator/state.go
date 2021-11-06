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
	"sync"
)

type NodeState struct {
	peerID         string
	connections    map[int]string
	ascendingPeer  string
	descendingPeer string
}

func NewNodeState(peerID string) *NodeState {
	node := &NodeState{
		peerID:         peerID,
		connections:    make(map[int]string),
		ascendingPeer:  "",
		descendingPeer: "",
	}
	return node
}

type State struct {
	nodes map[string]*NodeState
}

func NewState() *State {
	state := &State{
		nodes: make(map[string]*NodeState),
	}
	return state
}

type StateAccessor struct {
	m           *sync.Mutex
	subscribers []chan<- SimEvent

	state *State
}

func NewStateAccessor() *StateAccessor {
	sa := &StateAccessor{
		m:     &sync.Mutex{},
		state: NewState(),
	}
	return sa
}

func (s *StateAccessor) Subscribe(ch chan<- SimEvent) State {
	s.m.Lock()
	s.subscribers = append(s.subscribers, ch)
	stateCopy := *s.state
	s.m.Unlock()
	return stateCopy
}

func (s *StateAccessor) GetNodePeers() map[string][]string {
	s.m.Lock()
	defer s.m.Unlock()

	conns := make(map[string][]string)
	for name, node := range s.state.nodes {
		var nodeConns []string
		for _, conn := range node.connections {
			nodeConns = append(nodeConns, conn)
		}
		conns[name] = nodeConns
	}
	return conns
}

func (s *StateAccessor) GetSnakeNeighbours() map[string][]string {
	s.m.Lock()
	defer s.m.Unlock()

	peers := make(map[string][]string)
	for name, node := range s.state.nodes {
		var nodeConns []string
		nodeConns = append(nodeConns, node.ascendingPeer)
		nodeConns = append(nodeConns, node.descendingPeer)
		peers[name] = nodeConns
	}
	return peers
}

func (s *StateAccessor) GetNodeName(peerID string) (string, error) {
	for k, v := range s.state.nodes {
		if v.peerID == peerID {
			return k, nil
		}
	}
	return "", fmt.Errorf("Provided peerID is not associated with a known node")
}

func (s *StateAccessor) AddNode(name string, peerID string) {
	s.m.Lock()
	defer s.m.Unlock()

	s.state.nodes[name] = NewNodeState(peerID)
	s.publish(NodeAdded{Node: name})
}

func (s *StateAccessor) AddPeerConnection(from string, to string, port int) {
	s.m.Lock()
	defer s.m.Unlock()

	if _, ok := s.state.nodes[from]; ok {
		s.state.nodes[from].connections[port] = to
	}
	s.publish(PeerAdded{Node: from, Peer: to})
}

func (s *StateAccessor) RemovePeerConnection(from string, to string, port int) {
	s.m.Lock()
	defer s.m.Unlock()

	if _, ok := s.state.nodes[from]; ok {
		delete(s.state.nodes[from].connections, port)
	}
	s.publish(PeerRemoved{Node: from, Peer: to})
}

func (s *StateAccessor) UpdateAscendingPeer(node string, peerID string) {
	s.m.Lock()
	defer s.m.Unlock()

	if _, ok := s.state.nodes[node]; ok {
		prev := s.state.nodes[node].ascendingPeer
		s.state.nodes[node].ascendingPeer = peerID

		s.publish(SnakeAscUpdate{Node: node, Peer: peerID, Prev: prev})
	}
}

func (s *StateAccessor) UpdateDescendingPeer(node string, peerID string) {
	s.m.Lock()
	defer s.m.Unlock()

	if _, ok := s.state.nodes[node]; ok {
		prev := s.state.nodes[node].descendingPeer
		s.state.nodes[node].descendingPeer = peerID

		s.publish(SnakeDescUpdate{Node: node, Peer: peerID, Prev: prev})
	}
}

func (s *StateAccessor) publish(event SimEvent) {
	for _, subscriber := range s.subscribers {
		subscriber <- event
	}
}
