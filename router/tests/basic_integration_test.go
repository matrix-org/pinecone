// copyright 2022 the matrix.org foundation c.i.c.
//
// licensed under the apache license, version 2.0 (the "license");
// you may not use this file except in compliance with the license.
// you may obtain a copy of the license at
//
//     http://www.apache.org/licenses/license-2.0
//
// unless required by applicable law or agreed to in writing, software
// distributed under the license is distributed on an "as is" basis,
// without warranties or conditions of any kind, either express or implied.
// see the license for the specific language governing permissions and
// limitations under the license.

package integration

import (
	"testing"
	"time"
)

const SettlingTime time.Duration = time.Second * 2
const TestTimeout time.Duration = time.Second * 5

func TestNodesAgreeOnCorrectTreeRoot(t *testing.T) {
	t.Parallel()
	// Arrange
	scenario := NewScenarioFixture(t)
	nodes := []string{"Alice", "Bob", "Charlie"}
	scenario.AddStandardNodes(nodes)

	// Act
	scenario.AddPeerConnections([]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}})

	// Assert
	scenario.Validate(createTreeStateCapture(nodes), nodesAgreeOnCorrectTreeRoot, SettlingTime, TestTimeout)
}

func TestNodesAgreeOnCorrectSnakeFormation(t *testing.T) {
	t.Parallel()
	// Arrange
	scenario := NewScenarioFixture(t)
	nodes := []string{"Alice", "Bob", "Charlie"}
	scenario.AddStandardNodes(nodes)

	// Act
	scenario.AddPeerConnections([]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}})

	// Assert
	scenario.Validate(createSnakeStateCapture(nodes), nodesAgreeOnCorrectSnakeFormation, SettlingTime, TestTimeout)
}
