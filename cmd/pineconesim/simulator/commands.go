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
	"strconv"
	"time"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator/adversary"
	"github.com/matrix-org/pinecone/types"
)

const FAILURE_PREAMBLE = "Failed unmarshalling event: "

func UnmarshalCommandJSON(command *SimCommandMsg) (SimCommand, error) {
	var msg SimCommand
	var err error = nil
	switch command.MsgID {
	case SimDebug:
		msg = StateDebug{}
	case SimPlay:
		msg = Play{}
	case SimPause:
		msg = Pause{}
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
		nodeType := UnknownType
		if val, ok := command.Event.(map[string]interface{})["Name"]; ok {
			name = val.(string)
		} else {
			err = fmt.Errorf("%sAddNode.Name field doesn't exist", FAILURE_PREAMBLE)
		}

		if val, ok := command.Event.(map[string]interface{})["NodeType"]; ok {
			nodeType = APINodeType(val.(float64))
		} else {
			err = fmt.Errorf("%sAddNode.NodeType field doesn't exist", FAILURE_PREAMBLE)
		}
		msg = AddNode{name, nodeType}
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
	case SimConfigureAdversaryDefaults:
		node := ""
		dropRates := adversary.NewDropRates()
		if val, ok := command.Event.(map[string]interface{})["Node"]; ok {
			node = val.(string)
		} else {
			err = fmt.Errorf("%sConfigureAdversaryDefaults.Node field doesn't exist", FAILURE_PREAMBLE)
		}

		if val, ok := command.Event.(map[string]interface{})["DropRates"]; ok {
			if subVal, subOk := val.(map[string]interface{})["Overall"]; subOk {
				intVal, _ := strconv.Atoi(subVal.(string))
				dropRates.Overall = uint64(intVal)
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.Overall field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["Keepalive"]; subOk {
				intVal, _ := strconv.Atoi(subVal.(string))
				dropRates.Frames[types.TypeKeepalive] = uint64(intVal)
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.Keepalive field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeBootstrap"]; subOk {
				intVal, _ := strconv.Atoi(subVal.(string))
				dropRates.Frames[types.TypeVirtualSnakeBootstrap] = uint64(intVal)
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeBootstrap field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeRouted"]; subOk {
				intVal, _ := strconv.Atoi(subVal.(string))
				dropRates.Frames[types.TypeVirtualSnakeRouted] = uint64(intVal)
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeRouted field doesn't exist", FAILURE_PREAMBLE)
			}
		} else {
			err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates field doesn't exist", FAILURE_PREAMBLE)
		}
		msg = ConfigureAdversaryDefaults{node, dropRates}
	case SimConfigureAdversaryPeer:
		node := ""
		peer := ""
		dropRates := adversary.NewDropRates()
		if val, ok := command.Event.(map[string]interface{})["Node"]; ok {
			node = val.(string)
		} else {
			err = fmt.Errorf("%sConfigureAdversaryPeer.Node field doesn't exist", FAILURE_PREAMBLE)
		}
		if val, ok := command.Event.(map[string]interface{})["Peer"]; ok {
			peer = val.(string)
		} else {
			err = fmt.Errorf("%sConfigureAdversaryPeer.Peer field doesn't exist", FAILURE_PREAMBLE)
		}

		if val, ok := command.Event.(map[string]interface{})["DropRates"]; ok {
			if subVal, subOk := val.(map[string]interface{})["Overall"]; subOk {
				intVal, _ := strconv.Atoi(subVal.(string))
				dropRates.Overall = uint64(intVal)
			} else {
				err = fmt.Errorf("%sConfigureAdversaryPeer.DropRates.Overall field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["Keepalive"]; subOk {
				intVal, _ := strconv.Atoi(subVal.(string))
				dropRates.Frames[types.TypeKeepalive] = uint64(intVal)
			} else {
				err = fmt.Errorf("%sConfigureAdversaryPeer.DropRates.Keepalive field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeBootstrap"]; subOk {
				intVal, _ := strconv.Atoi(subVal.(string))
				dropRates.Frames[types.TypeVirtualSnakeBootstrap] = uint64(intVal)
			} else {
				err = fmt.Errorf("%sConfigureAdversaryPeer.DropRates.VirtualSnakeBootstrap field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeRouted"]; subOk {
				intVal, _ := strconv.Atoi(subVal.(string))
				dropRates.Frames[types.TypeVirtualSnakeRouted] = uint64(intVal)
			} else {
				err = fmt.Errorf("%sConfigureAdversaryPeer.DropRates.VirtualSnakeRouted field doesn't exist", FAILURE_PREAMBLE)
			}
		} else {
			err = fmt.Errorf("%sConfigureAdversaryPeer.DropRates field doesn't exist", FAILURE_PREAMBLE)
		}
		msg = ConfigureAdversaryPeer{node, peer, dropRates}
	case SimStartPings:
		msg = StartPings{}
	case SimStopPings:
		msg = StopPings{}
	default:
		err = fmt.Errorf("%sUnknown Event ID=%v", FAILURE_PREAMBLE, command.MsgID)
	}

	return msg, err
}

type SimCommand interface {
	Run(log *log.Logger, sim *Simulator)
}

type StateDebug struct{}

// Tag StateDebug as a Command
func (c StateDebug) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	log.Printf("Debug State: %s", sim.State.DebugLog())
}

func (c StateDebug) String() string {
	return "StateDebug{}"
}

type Play struct{}

// Tag Play as a Command
func (c Play) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	sim.Play()
}

func (c Play) String() string {
	return "Play{}"
}

type Pause struct{}

// Tag Pause as a Command
func (c Pause) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	sim.Pause()
}

func (c Pause) String() string {
	return "Pause{}"
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
	Node     string
	NodeType APINodeType
}

// Tag AddNode as a Command
func (c AddNode) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	if err := sim.CreateNode(c.Node, c.NodeType); err != nil {
		log.Printf("Failed creating new node %s: %s", c.Node, err)
		return
	}

	sim.StartNodeEventHandler(c.Node, c.NodeType)
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
	sim.RemoveNode(c.Node)
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
	if err := sim.ConnectNodes(c.Node, c.Peer); err != nil {
		log.Printf("Failed connecting node %s to node %s: %s", c.Node, c.Peer, err)
	}
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
	if err := sim.DisconnectNodes(c.Node, c.Peer); err != nil {
		log.Printf("Failed disconnecting node %s and node %s: %s", c.Node, c.Peer, err)
	}
}

func (c RemovePeer) String() string {
	return fmt.Sprintf("RemovePeer{Node:%s, Peer:%s}", c.Node, c.Peer)
}

type ConfigureAdversaryDefaults struct {
	Node      string
	DropRates adversary.DropRates
}

// Tag ConfigureAdversaryDefaults as an Command
func (c ConfigureAdversaryDefaults) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	sim.ConfigureFilterDefaults(c.Node, c.DropRates)
}

func (c ConfigureAdversaryDefaults) String() string {
	return fmt.Sprintf("ConfigureAdversaryDefaults{Node:%s, DropRates:%v}", c.Node, c.DropRates)
}

type ConfigureAdversaryPeer struct {
	Node      string
	Peer      string
	DropRates adversary.DropRates
}

// Tag ConfigureAdversaryPeer as an Command
func (c ConfigureAdversaryPeer) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	sim.ConfigureFilterPeer(c.Node, c.Peer, c.DropRates)
}

func (c ConfigureAdversaryPeer) String() string {
	return fmt.Sprintf("ConfigureAdversaryPeer{Node:%s, Peer:%s, DropRates:%v}", c.Node, c.Peer, c.DropRates)
}

type StartPings struct{}

// Tag StartPings as a Command
func (c StartPings) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	sim.StartPings()
}

func (c StartPings) String() string {
	return "StartPings{}"
}

type StopPings struct{}

// Tag StopPings as a Command
func (c StopPings) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	sim.StopPings()
}

func (c StopPings) String() string {
	return "StopPings{}"
}
