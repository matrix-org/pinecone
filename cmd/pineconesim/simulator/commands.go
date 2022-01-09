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
		dropRates := DropRates{}
		if val, ok := command.Event.(map[string]interface{})["Node"]; ok {
			node = val.(string)
		} else {
			err = fmt.Errorf("%sConfigureAdversaryDefaults.Node field doesn't exist", FAILURE_PREAMBLE)
		}

		if val, ok := command.Event.(map[string]interface{})["DropRates"]; ok {
			if subVal, subOk := val.(map[string]interface{})["Keepalive"]; subOk {
				dropRates.KeepAlive, _ = strconv.Atoi(subVal.(string))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.Keepalive field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["TreeAnnouncement"]; subOk {
				dropRates.TreeAnnouncement, _ = strconv.Atoi(subVal.(string))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.TreeAnnouncement field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["TreeRouted"]; subOk {
				dropRates.TreeRouted, _ = strconv.Atoi(subVal.(string))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.TreeRouted field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeBootstrap"]; subOk {
				dropRates.VirtualSnakeBootstrap, _ = strconv.Atoi(subVal.(string))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeBootstrap field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeBootstrapACK"]; subOk {
				dropRates.VirtualSnakeBootstrapACK, _ = strconv.Atoi(subVal.(string))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeBootstrapACK field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeSetup"]; subOk {
				dropRates.VirtualSnakeSetup, _ = strconv.Atoi(subVal.(string))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeSetup field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeSetupACK"]; subOk {
				dropRates.VirtualSnakeSetupACK, _ = strconv.Atoi(subVal.(string))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeSetupACK field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeTeardown"]; subOk {
				dropRates.VirtualSnakeTeardown, _ = strconv.Atoi(subVal.(string))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeTeardown field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeRouted"]; subOk {
				dropRates.VirtualSnakeRouted, _ = strconv.Atoi(subVal.(string))
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
		dropRates := DropRates{}
		if val, ok := command.Event.(map[string]interface{})["Node"]; ok {
			node = val.(string)
		} else {
			err = fmt.Errorf("%sConfigureAdversaryDefaults.Node field doesn't exist", FAILURE_PREAMBLE)
		}
		if val, ok := command.Event.(map[string]interface{})["Peer"]; ok {
			peer = val.(string)
		} else {
			err = fmt.Errorf("%sConfigureAdversaryDefaults.Peer field doesn't exist", FAILURE_PREAMBLE)
		}

		if val, ok := command.Event.(map[string]interface{})["DropRates"]; ok {
			if subVal, subOk := val.(map[string]interface{})["Keepalive"]; subOk {
				dropRates.KeepAlive = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.Keepalive field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["TreeAnnouncement"]; subOk {
				dropRates.TreeAnnouncement = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.TreeAnnouncement field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["TreeRouted"]; subOk {
				dropRates.TreeRouted = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.TreeRouted field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeBootstrap"]; subOk {
				dropRates.VirtualSnakeBootstrap = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeBootstrap field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeBootstrapACK"]; subOk {
				dropRates.VirtualSnakeBootstrapACK = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeBootstrapACK field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeSetup"]; subOk {
				dropRates.VirtualSnakeSetup = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeSetup field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeSetupACK"]; subOk {
				dropRates.VirtualSnakeSetupACK = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeSetupACK field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeTeardown"]; subOk {
				dropRates.VirtualSnakeTeardown = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeTeardown field doesn't exist", FAILURE_PREAMBLE)
			}
			if subVal, subOk := val.(map[string]interface{})["VirtualSnakeRouted"]; subOk {
				dropRates.VirtualSnakeRouted = int(subVal.(float64))
			} else {
				err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates.VirtualSnakeRouted field doesn't exist", FAILURE_PREAMBLE)
			}
		} else {
			err = fmt.Errorf("%sConfigureAdversaryDefaults.DropRates field doesn't exist", FAILURE_PREAMBLE)
		}
		msg = ConfigureAdversaryPeer{node, peer, dropRates}
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
	if err := sim.CreateNode(c.Node); err != nil {
		log.Printf("Failed creating new node %s: %s", c.Node, err)
		return
	}

	sim.StartNodeEventHandler(c.Node)
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

type DropRates struct {
	KeepAlive                int
	TreeAnnouncement         int
	TreeRouted               int
	VirtualSnakeBootstrap    int
	VirtualSnakeBootstrapACK int
	VirtualSnakeSetup        int
	VirtualSnakeSetupACK     int
	VirtualSnakeTeardown     int
	VirtualSnakeRouted       int
}

type ConfigureAdversaryDefaults struct {
	Node      string
	DropRates DropRates
}

// Tag ConfigureAdversaryDefaults as an Command
func (c ConfigureAdversaryDefaults) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	// TODO
}

func (c ConfigureAdversaryDefaults) String() string {
	return fmt.Sprintf("ConfigureAdversaryDefaults{Node:%s, DropRates:%v}", c.Node, c.DropRates)
}

type ConfigureAdversaryPeer struct {
	Node      string
	Peer      string
	DropRates DropRates
}

// Tag ConfigureAdversaryPeer as an Command
func (c ConfigureAdversaryPeer) Run(log *log.Logger, sim *Simulator) {
	log.Printf("Executing command %s", c)
	// TODO
}

func (c ConfigureAdversaryPeer) String() string {
	return fmt.Sprintf("ConfigureAdversaryPeer{Node:%s, Peer:%s, DropRates:%v}", c.Node, c.Peer, c.DropRates)
}
