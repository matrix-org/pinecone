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

package integration

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator/adversary"
	"github.com/matrix-org/pinecone/types"
)

var keyLowest = ed25519.PrivateKey{150, 150, 180, 13, 144, 213, 73, 184, 153, 38, 175, 150, 74, 58, 119, 123, 76, 148, 143, 63, 27, 138, 34, 91, 7, 38, 158, 206, 187, 106, 5, 206, 20, 81, 99, 14, 255, 61, 112, 173, 221, 15, 128, 30, 180, 215, 21, 109, 186, 3, 122, 27, 48, 140, 154, 155, 180, 71, 40, 161, 53, 169, 178, 140}
var keyMidOne = ed25519.PrivateKey{250, 250, 106, 34, 162, 70, 252, 104, 244, 18, 56, 2, 223, 2, 28, 64, 149, 194, 91, 54, 85, 52, 202, 74, 75, 200, 2, 101, 29, 37, 206, 40, 38, 137, 220, 170, 86, 99, 138, 172, 211, 245, 221, 195, 61, 62, 222, 18, 80, 19, 236, 194, 157, 70, 200, 159, 246, 207, 149, 55, 172, 29, 79, 49}
var keyMidTwo = ed25519.PrivateKey{200, 200, 47, 30, 166, 161, 173, 147, 101, 144, 249, 22, 165, 179, 104, 35, 58, 85, 45, 118, 228, 204, 236, 113, 82, 246, 216, 61, 240, 42, 198, 185, 105, 79, 204, 91, 148, 97, 125, 43, 194, 168, 128, 141, 110, 94, 108, 246, 60, 64, 200, 37, 31, 175, 4, 155, 178, 130, 210, 149, 21, 184, 152, 116}
var keyHighest = ed25519.PrivateKey{255, 255, 22, 151, 14, 92, 84, 175, 249, 1, 162, 19, 90, 51, 235, 125, 144, 252, 195, 128, 133, 219, 115, 134, 235, 245, 47, 201, 95, 100, 24, 28, 221, 105, 40, 142, 161, 4, 70, 1, 88, 44, 10, 118, 228, 71, 67, 0, 108, 195, 10, 244, 32, 186, 117, 77, 54, 109, 163, 23, 245, 161, 89, 211}

func TestNodesAgreeOnCorrectTreeRootAdversaryNoDrops(t *testing.T) {
	t.Parallel()
	// Arrange
	scenario := NewScenarioFixture(t)
	genericNodes := []string{"Alice", "Bob", "Charlie"}
	scenario.AddStandardNodes(genericNodes)
	adversaries := []string{"Mallory"}
	scenario.AddAdversaryNodes(adversaries)

	defaultDropRates := adversary.NewDropRates()
	peerDropRates := map[string]adversary.DropRates{}
	scenario.ConfigureAdversary("Mallory", defaultDropRates, peerDropRates)

	// Act
	scenario.AddPeerConnections([]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Charlie", "Mallory"}})

	// Assert
	nodesExpectedToConverge := append(genericNodes, adversaries...)
	scenario.Validate(createTreeStateCapture(nodesExpectedToConverge), nodesAgreeOnCorrectTreeRoot, 2*time.Second, 5*time.Second)
}

func TestNodesAgreeOnCorrectTreeRootAdversaryDropTreeAnnouncements(t *testing.T) {
	t.Parallel()
	// Arrange
	scenario := NewScenarioFixture(t)
	genericNodes := []string{"Alice", "Bob", "Charlie"}
	scenario.AddStandardNodes(genericNodes)

	adversaries := []string{"Mallory"}
	scenario.AddAdversaryNodes(adversaries)

	defaultDropRates := adversary.NewDropRates()
	defaultDropRates.Frames[types.TypeTreeAnnouncement] = 100
	peerDropRates := map[string]adversary.DropRates{}
	scenario.ConfigureAdversary("Mallory", defaultDropRates, peerDropRates)

	// Act
	scenario.AddPeerConnections([]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Charlie", "Mallory"}})

	// Assert
	nodesExpectedToConverge := (genericNodes)
	scenario.Validate(createTreeStateCapture(nodesExpectedToConverge), nodesAgreeOnCorrectTreeRoot, 2*time.Second, 5*time.Second)
}

func TestNodesAgreeOnCorrectTreeRootAdversaryDropSnakeHighestKey(t *testing.T) {
	t.Parallel()
	// Arrange
	scenario := NewScenarioFixture(t)
	genericNodes := map[string]*ed25519.PrivateKey{"Alice": &keyLowest, "Bob": &keyMidOne, "Charlie": &keyMidTwo}
	scenario.AddStandardNodesWithKeys(genericNodes)

	adversaries := map[string]*ed25519.PrivateKey{"Mallory": &keyHighest}
	scenario.AddAdversaryNodesWithKeys(adversaries)

	defaultDropRates := adversary.NewDropRates()
	defaultDropRates.Frames[types.TypeVirtualSnakeBootstrap] = 100
	peerDropRates := map[string]adversary.DropRates{}
	scenario.ConfigureAdversary("Mallory", defaultDropRates, peerDropRates)

	// Act
	scenario.AddPeerConnections([]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Charlie", "Mallory"}})

	// Assert
	nodes := make([]string, 0, len(genericNodes))
	for node := range genericNodes {
		nodes = append(nodes, node)
	}
	nodesExpectedToConverge := nodes
	scenario.Validate(createSnakeStateCapture(nodesExpectedToConverge), nodesAgreeOnCorrectSnakeFormation, 2*time.Second, 5*time.Second)
}
