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

//go:build test_adversary
// +build test_adversary

package integration

import (
	"crypto/ed25519"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/pinecone/cmd/pineconesim/simulator/adversary"
	"github.com/matrix-org/pinecone/types"
)

var keyAA = ed25519.PrivateKey{162, 26, 23, 81, 135, 60, 226, 48, 76, 67, 143, 253, 156, 159, 49, 135, 0, 98, 62, 97, 233, 228, 220, 156, 6, 159, 71, 51, 158, 168, 98, 248, 170, 170, 180, 9, 248, 84, 20, 240, 141, 118, 183, 23, 7, 120, 31, 109, 72, 88, 6, 104, 51, 155, 117, 44, 50, 157, 81, 104, 18, 73, 249, 94}
var keyBB = ed25519.PrivateKey{126, 39, 115, 163, 236, 170, 93, 74, 222, 226, 27, 212, 18, 17, 52, 167, 199, 150, 49, 224, 81, 21, 77, 182, 245, 137, 255, 250, 142, 60, 57, 29, 187, 187, 168, 181, 14, 162, 182, 255, 57, 139, 163, 231, 33, 194, 45, 112, 112, 137, 68, 12, 23, 50, 62, 47, 208, 49, 37, 65, 112, 114, 153, 215}
var keyCC = ed25519.PrivateKey{88, 94, 100, 40, 136, 89, 209, 11, 241, 92, 136, 72, 232, 113, 52, 96, 173, 4, 73, 213, 223, 110, 222, 181, 169, 63, 140, 37, 169, 202, 227, 181, 204, 204, 102, 150, 191, 219, 16, 52, 178, 118, 12, 115, 3, 167, 138, 52, 255, 183, 159, 124, 134, 166, 167, 137, 65, 168, 16, 196, 41, 127, 21, 206}
var keyDD = ed25519.PrivateKey{70, 46, 50, 7, 34, 2, 71, 228, 187, 108, 175, 125, 158, 249, 114, 239, 111, 86, 10, 167, 27, 199, 131, 111, 50, 252, 162, 178, 37, 150, 219, 54, 221, 221, 113, 44, 216, 192, 87, 137, 164, 138, 177, 86, 229, 65, 246, 146, 191, 204, 107, 219, 4, 102, 213, 251, 71, 255, 51, 133, 168, 124, 30, 224}
var keyEE = ed25519.PrivateKey{8, 74, 248, 201, 148, 80, 73, 116, 112, 92, 53, 189, 238, 60, 68, 7, 18, 107, 63, 201, 29, 167, 243, 27, 21, 227, 204, 90, 225, 16, 113, 143, 238, 238, 73, 38, 91, 255, 177, 179, 179, 151, 55, 84, 129, 232, 94, 187, 42, 206, 159, 99, 249, 146, 32, 0, 133, 61, 101, 91, 77, 9, 179, 158}
var keyFF = ed25519.PrivateKey{110, 213, 18, 180, 196, 140, 66, 142, 132, 208, 11, 196, 242, 179, 47, 227, 153, 156, 76, 146, 254, 72, 89, 93, 42, 134, 28, 64, 153, 61, 18, 246, 255, 255, 144, 7, 51, 1, 43, 177, 12, 48, 76, 248, 107, 197, 3, 223, 189, 198, 162, 38, 4, 122, 61, 58, 142, 251, 63, 162, 33, 134, 9, 141}

var sortedKeys = []ed25519.PrivateKey{keyAA, keyBB, keyCC, keyDD, keyEE, keyFF}

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
	scenario.ConnectNodes([]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Charlie", "Mallory"}})

	// Assert
	nodesExpectedToConverge := (genericNodes)
	scenario.Validate(createTreeStateCapture(nodesExpectedToConverge), nodesAgreeOnCorrectTreeRoot, 2*time.Second, 5*time.Second)
}

type TestCase struct {
	desc         string
	genericNodes []string
	adversaries  []string
	connections  []NodePair
	framesToDrop []types.FrameType
}

type KeyMap map[string]*ed25519.PrivateKey

type SystemUnderTest struct {
	scenario        ScenarioFixture
	genericNodeKeys KeyMap
	adversaryKeys   KeyMap
}

func TestNodesCanPingAdversaryDropsSnakeBootstraps(t *testing.T) {
	t.Parallel()

	cases := []TestCase{
		{
			"Test2NodeLineSNEKBootstrap",
			[]string{"Alice", "Bob"},
			[]string{"Mallory"},
			[]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Alice", "Mallory"}},
			[]types.FrameType{types.TypeVirtualSnakeBootstrap},
		},
		{
			"Test3NodeLineSNEKBootstrap",
			[]string{"Alice", "Bob", "Charlie"},
			[]string{"Mallory"},
			[]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Alice", "Mallory"}},
			[]types.FrameType{types.TypeVirtualSnakeBootstrap},
		},
		{
			"Test3NodeLineSNEKBootstrapACK",
			[]string{"Alice", "Bob", "Charlie"},
			[]string{"Mallory"},
			[]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Alice", "Mallory"}},
			[]types.FrameType{types.TypeVirtualSnakeBootstrapACK},
		},
		{
			"Test3NodeLineSNEKSetup",
			[]string{"Alice", "Bob", "Charlie"},
			[]string{"Mallory"},
			[]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Alice", "Mallory"}},
			[]types.FrameType{types.TypeVirtualSnakeSetup},
		},
		{
			"Test3NodeLineSNEKSetupACK",
			[]string{"Alice", "Bob", "Charlie"},
			[]string{"Mallory"},
			[]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Alice", "Mallory"}},
			[]types.FrameType{types.TypeVirtualSnakeSetupACK},
		},
		{
			"Test3NodeTriangleSNEKBootstrap",
			[]string{"Alice", "Bob", "Charlie"},
			[]string{"Mallory"},
			[]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Charlie", "Alice"}, NodePair{"Alice", "Mallory"}},
			[]types.FrameType{types.TypeVirtualSnakeBootstrap},
		},
		{
			"Test4NodeFanSNEKBootstrap",
			[]string{"Alice", "Bob", "Charlie", "Dan"},
			[]string{"Mallory"},
			[]NodePair{NodePair{"Alice", "Bob"}, NodePair{"Bob", "Charlie"}, NodePair{"Bob", "Dan"}, NodePair{"Dan", "Alice"}, NodePair{"Alice", "Mallory"}},
			[]types.FrameType{types.TypeVirtualSnakeBootstrap},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			totalNodes := len(tc.genericNodes) + len(tc.adversaries)
			if totalNodes > len(sortedKeys) {
				t.Fatalf("Too many nodes specified in the test case. There are only %d keys available.", len(sortedKeys))
			}

			keySubset := []*ed25519.PrivateKey{}
			for i := 0; i < totalNodes; i++ {
				keySubset = append(keySubset, &sortedKeys[i])
			}
			keyPermutations := permutations(keySubset)

			for i, keyOrder := range keyPermutations {
				i, keyOrder := i, keyOrder
				t.Run(fmt.Sprintf("KeyPermutation#%d", i), func(t *testing.T) {
					t.Parallel()

					log.Printf("Starting Test Case: %s", tc.desc)
					// Arrange
					sut := createSystemUnderTest(t, tc, keyOrder, []types.FrameType{types.TypeVirtualSnakeBootstrap})

					// Act
					sut.scenario.ConnectNodes(tc.connections)

					// Assert
					sut.scenario.Validate(createTreeStateCapture(tc.genericNodes), nodesAgreeOnCorrectTreeRoot, SettlingTime, TestTimeout)
					if !nodesCanAllPingEachOther(&sut.scenario, tc.genericNodes) {
						printSystemUnderTest(tc, &sut)
					}
				})
			}
		})
	}
}

func createSystemUnderTest(t *testing.T, tc TestCase, keyOrder []*ed25519.PrivateKey, dropTypes []types.FrameType) SystemUnderTest {
	fixture := NewScenarioFixture(t)
	keyIndex := 0
	genericNodeKeys := KeyMap{}
	for _, node := range tc.genericNodes {
		genericNodeKeys[node] = keyOrder[keyIndex]
		keyIndex++
	}
	fixture.AddStandardNodesWithKeys(genericNodeKeys)

	adversaryKeys := KeyMap{}
	for _, node := range tc.adversaries {
		adversaryKeys[node] = keyOrder[keyIndex]
		keyIndex++
	}
	fixture.AddAdversaryNodesWithKeys(adversaryKeys)

	defaultDropRates := adversary.NewDropRates()
	for _, frameType := range dropTypes {
		defaultDropRates.Frames[frameType] = 100
	}
	peerDropRates := map[string]adversary.DropRates{}
	for _, adversary := range tc.adversaries {
		fixture.ConfigureAdversary(adversary, defaultDropRates, peerDropRates)
	}

	return SystemUnderTest{fixture, genericNodeKeys, adversaryKeys}
}

func printSystemUnderTest(tc TestCase, sut *SystemUnderTest) {
	keys := map[string]string{}
	for k, v := range sut.genericNodeKeys {
		var sk types.PrivateKey
		copy(sk[:], *v)
		keys[k] = strings.ToUpper(sk.Public().String()[:8])
	}
	for k, v := range sut.adversaryKeys {
		var sk types.PrivateKey
		copy(sk[:], *v)
		keys[k] = strings.ToUpper(sk.Public().String()[:8])
	}

	sut.scenario.t.Errorf("Test Parameters: \nTest Case: %+v\nNode Keys: %+v", tc, keys)
}

func permutations(xs []*ed25519.PrivateKey) (permuts [][]*ed25519.PrivateKey) {
	var rc func([]*ed25519.PrivateKey, int)
	rc = func(a []*ed25519.PrivateKey, k int) {
		if k == len(a) {
			permuts = append(permuts, append([]*ed25519.PrivateKey{}, a...))
		} else {
			for i := k; i < len(xs); i++ {
				a[k], a[i] = a[i], a[k]
				rc(a, k+1)
				a[k], a[i] = a[i], a[k]
			}
		}
	}
	rc(xs, 0)

	return permuts
}
