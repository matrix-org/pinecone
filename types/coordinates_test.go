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

package types

import (
	"bytes"
	"testing"
)

func TestSwitchPorts(t *testing.T) {
	var b [7]byte
	expected := []byte{0, 5, 1, 2, 3, 159, 32}
	input := Coordinates{1, 2, 3, 4000}
	_, err := input.MarshalBinary(b[:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b[:], expected) {
		t.Fatalf("MarshalBinary produced %v, expected %v", b, expected)
	}
	var output Coordinates
	if _, err := output.UnmarshalBinary(b[:]); err != nil {
		t.Fatal(err)
	}
	if !input.EqualTo(output) {
		t.Fatalf("Expected %v, got %v", input, output)
	}
}

func TestSwitchPortDistances(t *testing.T) {
	root := Coordinates{}
	parent := Coordinates{1, 2, 3}
	us := Coordinates{1, 2, 3, 4}
	other := Coordinates{1, 3, 3, 4, 7, 6, 1}
	if dist := us.DistanceTo(root); dist != 4 {
		t.Fatalf("distance from us to root should be 4, got %d", dist)
	}
	if dist := parent.DistanceTo(root); dist != 3 {
		t.Fatalf("distance from parent to root should be 3, got %d", dist)
	}
	if dist := root.DistanceTo(us); dist != 4 {
		t.Fatalf("distance from root to us should be 4, got %d", dist)
	}
	if dist := root.DistanceTo(parent); dist != 3 {
		t.Fatalf("distance from root to parent should be 3, got %d", dist)
	}
	if dist := us.DistanceTo(other); dist != 9 {
		t.Fatalf("distance from us to other should be 9, got %d", dist)
	}
	if dist := parent.DistanceTo(other); dist != 8 {
		t.Fatalf("distance from parent to other should be 8, got %d", dist)
	}
	if dist := root.DistanceTo(other); dist != 7 {
		t.Fatalf("distance from root to other should be 7, got %d", dist)
	}
}
