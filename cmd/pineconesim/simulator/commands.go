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
	"log"
	"time"
)

type APICommandID int

const (
	UnknownCommand APICommandID = iota
	SimDelay
	SimAddNode
	SimRemoveNode
	SimAddPeer
	SimRemovePeer
)

const FAILURE_PREAMBLE = "Failed unmarshalling event: "

func UnmarshalCommandJSON(command *SimCommandMsg) (SimCommand, error) {
	var msg SimCommand
	var err error = nil
	switch command.MsgID {
	case SimDelay:
		length := uint64(0)
		if val, ok := command.Event.(map[string]interface{})["Length"]; ok {
			length = uint64(val.(float64))
		} else {
			err = fmt.Errorf("%sDelay.Length field doesn't exist", FAILURE_PREAMBLE)
		}
		msg = Delay{length}
	case SimAddNode:
		name := ""
		if val, ok := command.Event.(map[string]interface{})["Name"]; ok {
			name = val.(string)
		} else {
			err = fmt.Errorf("%sAddNode.Name field doesn't exist", FAILURE_PREAMBLE)
		}
		msg = AddNode{name}
	case SimRemoveNode:
		name := ""
		if val, ok := command.Event.(map[string]interface{})["Name"]; ok {
			name = val.(string)
		} else {
			err = fmt.Errorf("%sRemoveNode.Name field doesn't exist", FAILURE_PREAMBLE)
		}
		msg = RemoveNode{name}

	case SimAddPeer:
		node := ""
		peer := ""
		if val, ok := command.Event.(map[string]interface{})["Node"]; ok {
			node = val.(string)
		} else {
			err = fmt.Errorf("%sAddPeer.Node field doesn't exist", FAILURE_PREAMBLE)
		}

		if val, ok := command.Event.(map[string]interface{})["Peer"]; ok {
			peer = val.(string)
		} else {
			err = fmt.Errorf("%sAddPeer.Peer field doesn't exist", FAILURE_PREAMBLE)
		}

		msg = AddPeer{node, peer}
	case SimRemovePeer:
		node := ""
		peer := ""
		if val, ok := command.Event.(map[string]interface{})["Node"]; ok {
			node = val.(string)
		} else {
			err = fmt.Errorf("%sRemoveNode.Node field doesn't exist", FAILURE_PREAMBLE)
		}

		if val, ok := command.Event.(map[string]interface{})["Peer"]; ok {
			peer = val.(string)
		} else {
			err = fmt.Errorf("%sRemoveNode.Peer field doesn't exist", FAILURE_PREAMBLE)
		}

		msg = RemovePeer{node, peer}

	default:
		err = fmt.Errorf("%sUnknown Event ID=%v", FAILURE_PREAMBLE, command.MsgID)
	}

	return msg, err
}

type SimCommand interface {
	Run(log *log.Logger, sim *Simulator)
}

type Delay struct {
	Length uint64 // delay time in ms
}

// Tag Delay as a Command
func (c Delay) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	time.Sleep(time.Duration(c.Length) * time.Millisecond)
}

func (c Delay) String() string {
	return fmt.Sprintf("Delay{Length:%d}", c.Length)
}

type AddNode struct {
	Node string
}

// Tag AddNode as a Command
func (c AddNode) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	if _, ok := sim.nodes[c.Node]; ok {
		log.Printf("Node %s already exists!", c.Node)
	} else {
		if err := sim.CreateNode(c.Node); err != nil {
			log.Printf("Failed creating new node %s", c.Node)
			return
		}

		sim.StartNodeEventHandler(c.Node)
	}
}

func (c AddNode) String() string {
	return fmt.Sprintf("AddNode{Name:%s}", c.Node)
}

type RemoveNode struct {
	Node string
}

// Tag RemoveNode as an Command
func (c RemoveNode) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	// TODO
}

func (c RemoveNode) String() string {
	return fmt.Sprintf("RemoveNode{Name:%s}", c.Node)
}

type AddPeer struct {
	Node string
	Peer string
}

// Tag AddPeer as a Command
func (c AddPeer) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	// TODO
}

func (c AddPeer) String() string {
	return fmt.Sprintf("AddPeer{Node:%s, Peer:%s}", c.Node, c.Peer)
}

type RemovePeer struct {
	Node string
	Peer string
}

// Tag RemovePeer as an Command
func (c RemovePeer) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	// TODO
}

func (c RemovePeer) String() string {
	return fmt.Sprintf("RemovePeer{Node:%s, Peer:%s}", c.Node, c.Peer)
}