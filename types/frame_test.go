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
	"crypto/ed25519"
	"fmt"
	"testing"
)

func TestMarshalUnmarshalFrame(t *testing.T) {
	input := Frame{
		Version:     Version0,
		Type:        TypeGreedy,
		Destination: SwitchPorts{1, 2, 3, 4, 5000},
		Source:      SwitchPorts{4, 3, 2, 1},
		Payload:     []byte("ABCDEFG"),
	}
	expected := []byte{
		0x70, 0x69, 0x6e, 0x65, // magic bytes
		0,     // version 0
		2,     // type greedy
		0, 35, // frame length
		0, 8, // destination len
		0, 6, // source len
		0, 7, // payload len
		0, 6, 1, 2, 3, 4, 167, 8, // destination (2+6 bytes but 5 ports!)
		0, 4, 4, 3, 2, 1, // source (2+4 bytes)
		65, 66, 67, 68, 69, 70, 71, // payload (7 bytes)
	}
	buf := make([]byte, 65535)
	n, err := input.MarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(expected) {
		t.Fatalf("wrong marshalled length, got %d, expected %d", n, len(expected))
	}
	if !bytes.Equal(buf[:n], expected) {
		t.Fatalf("wrong marshalled output, got %v, expected %v", buf[:n], expected)
	}
	var output Frame
	if _, err := output.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if output.Version != input.Version {
		t.Fatal("wrong version")
	}
	if output.Type != input.Type {
		t.Fatal("wrong version")
	}
	if l := len(output.Destination); l != 5 {
		t.Fatalf("wrong destination length (got %d, expected 5)", l)
	}
	if l := len(output.Source); l != 4 {
		t.Fatalf("wrong source length (got %d, expected 4)", l)
	}
	if l := len(output.Payload); l != 7 {
		t.Fatalf("wrong payload length (got %d, expected 7)", l)
	}
	if !output.Destination.EqualTo(input.Destination) {
		t.Fatal("wrong path")
	}
	if !bytes.Equal(input.Payload, output.Payload) {
		t.Fatal("wrong payload")
	}
}

func TestMarshalUnmarshalSNEKBootstrapFrame(t *testing.T) {
	pk, _, _ := ed25519.GenerateKey(nil)
	input := Frame{
		Version: Version0,
		Type:    TypeVirtualSnakeBootstrap,
		Source:  SwitchPorts{1, 2, 3, 4, 5},
		Payload: []byte{9, 9, 9, 9, 9},
	}
	copy(input.DestinationKey[:], pk)
	expected := []byte{
		0x70, 0x69, 0x6e, 0x65, // magic bytes
		0,                               // version 0
		byte(TypeVirtualSnakeBootstrap), // type greedy
		0, 54,                           // frame length
		0, 5, // payload length
		0, 5, // source length
		1, 2, 3, 4, 5, // source coordinates
	}
	expected = append(expected, pk...)
	expected = append(expected, input.Payload...)
	buf := make([]byte, 65535)
	n, err := input.MarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(expected) {
		t.Fatalf("wrong marshalled length, got %d, expected %d", n, len(expected))
	}
	if !bytes.Equal(buf[:n], expected) {
		t.Fatalf("wrong marshalled output, got %v", buf[:n])
	}

	t.Log("Got: ", buf[:n])
	t.Log("Want:", expected)

	var output Frame
	if _, err := output.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if output.Version != input.Version {
		t.Fatal("wrong version")
	}
	if output.Type != input.Type {
		t.Fatal("wrong version")
	}
	if !output.Destination.EqualTo(input.Destination) {
		t.Fatal("wrong path")
	}
	if !bytes.Equal(input.Payload, output.Payload) {
		t.Fatal("wrong payload")
	}
}

func TestMarshalUnmarshalSNEKBootstrapACKFrame(t *testing.T) {
	pk1, _, _ := ed25519.GenerateKey(nil)
	pk2, _, _ := ed25519.GenerateKey(nil)
	input := Frame{
		Version:     Version0,
		Type:        TypeVirtualSnakeBootstrapACK,
		Source:      SwitchPorts{1, 2, 3, 4, 5},
		Destination: SwitchPorts{5, 4, 3, 2, 1},
		Payload:     []byte{9, 9, 9, 9, 9},
	}
	copy(input.DestinationKey[:], pk1)
	copy(input.SourceKey[:], pk2)
	expected := []byte{
		0x70, 0x69, 0x6e, 0x65, // magic bytes
		0,                                  // version 0
		byte(TypeVirtualSnakeBootstrapACK), // type greedy
		0, 97,                              // frame length
		0, 5, // payload length
		0, 7, // destination length
		0, 7, // source length
		0, 5, 5, 4, 3, 2, 1, // destination coordinates
		0, 5, 1, 2, 3, 4, 5, // source coordinates
	}
	expected = append(expected, pk1...)
	expected = append(expected, pk2...)
	expected = append(expected, input.Payload...)
	buf := make([]byte, 65535)
	n, err := input.MarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(expected) {
		t.Fatalf("wrong marshalled length, got %d, expected %d", n, len(expected))
	}
	if !bytes.Equal(buf[:n], expected) {
		t.Fatalf("wrong marshalled output, got %v, expected %v", buf[:n], expected)
	}

	t.Log("Got: ", buf[:n])
	t.Log("Want:", expected)

	var output Frame
	if _, err := output.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if output.Version != input.Version {
		t.Fatal("wrong version")
	}
	if output.Type != input.Type {
		t.Fatal("wrong version")
	}
	if !output.Destination.EqualTo(input.Destination) {
		t.Fatal("wrong path")
	}
	if !bytes.Equal(input.Payload, output.Payload) {
		t.Fatal("wrong payload")
	}
}

func TestMarshalUnmarshalSNEKSetupFrame(t *testing.T) {
	pk1, _, _ := ed25519.GenerateKey(nil)
	pk2, _, _ := ed25519.GenerateKey(nil)
	input := Frame{
		Version:     Version0,
		Type:        TypeVirtualSnakeSetup,
		Destination: SwitchPorts{5, 4, 3, 2, 1},
		Payload:     []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
	}
	copy(input.DestinationKey[:], pk1)
	copy(input.SourceKey[:], pk2)
	expected := []byte{
		0x70, 0x69, 0x6e, 0x65, // magic bytes
		0,                           // version 0
		byte(TypeVirtualSnakeSetup), // type greedy
		0, 91,                       // frame length
		0, 10, // payload length
		0, 5, 5, 4, 3, 2, 1, // destination coordinates
	}
	expected = append(expected, pk2...)
	expected = append(expected, pk1...)
	expected = append(expected, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9) // payload
	buf := make([]byte, 65535)
	n, err := input.MarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(expected) {
		t.Fatalf("wrong marshalled length, \ngot %d, \nexpected %d", n, len(expected))
	}
	if !bytes.Equal(buf[:n], expected) {
		t.Fatalf("wrong marshalled output, \ngot %v, \nexpected %v", buf[:n], expected)
	}

	t.Log("Got: ", buf[:n])
	t.Log("Want:", expected)

	var output Frame
	if _, err := output.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if output.Version != input.Version {
		t.Fatal("wrong version")
	}
	if output.Type != input.Type {
		t.Fatal("wrong version")
	}
	if !output.Destination.EqualTo(input.Destination) {
		t.Fatal("wrong path")
	}
	if !bytes.Equal(input.Payload, output.Payload) {
		t.Fatal("wrong payload")
	}
}

func TestMarshalUnmarshalSNEKTeardownFrame(t *testing.T) {
	pk1, _, _ := ed25519.GenerateKey(nil)
	input := Frame{
		Version: Version0,
		Type:    TypeVirtualSnakeTeardown,
	}
	copy(input.DestinationKey[:], pk1)
	expected := []byte{
		0x70, 0x69, 0x6e, 0x65, // magic bytes
		0,                              // version 0
		byte(TypeVirtualSnakeTeardown), // type greedy
		0, 42,                          // frame length
		0, 0, // payload length
	}
	expected = append(expected, pk1...)
	buf := make([]byte, 65535)
	n, err := input.MarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(expected) {
		t.Fatalf("wrong marshalled length, \ngot %d, \nexpected %d", n, len(expected))
	}
	if !bytes.Equal(buf[:n], expected) {
		t.Fatalf("wrong marshalled output, \ngot %v, \nexpected %v", buf[:n], expected)
	}

	t.Log("Got: ", buf[:n])
	t.Log("Want:", expected)

	var output Frame
	if _, err := output.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if output.Version != input.Version {
		t.Fatal("wrong version")
	}
	if output.Type != input.Type {
		t.Fatal("wrong version")
	}
	if !output.Destination.EqualTo(input.Destination) {
		t.Fatal("wrong path")
	}
	if !bytes.Equal(input.Payload, output.Payload) {
		t.Fatal("wrong payload")
	}
}

func TestMarshalUnmarshalSNEKFrame(t *testing.T) {
	pk1, _, _ := ed25519.GenerateKey(nil)
	pk2, _, _ := ed25519.GenerateKey(nil)
	input := Frame{
		Version: Version0,
		Type:    TypeVirtualSnake,
		Payload: []byte("HELLO!"),
	}
	copy(input.SourceKey[:], pk1)
	copy(input.DestinationKey[:], pk2)
	expected := []byte{
		0x70, 0x69, 0x6e, 0x65, // magic bytes
		0,                      // version 0
		byte(TypeVirtualSnake), // type greedy
		0, 80,                  // frame length
		0, 6, // payload length
	}
	expected = append(expected, pk2...)
	expected = append(expected, pk1...)
	expected = append(expected, input.Payload...)
	buf := make([]byte, 65535)
	n, err := input.MarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(expected) {
		t.Fatalf("wrong marshalled length, got %d, expected %d", n, len(expected))
	}
	if !bytes.Equal(buf[:n], expected) {
		fmt.Println("got: ", buf[:n])
		fmt.Println("want:", expected)
		t.Fatalf("wrong marshalled output")
	}

	var output Frame
	if _, err := output.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if output.Version != input.Version {
		t.Fatal("wrong version")
	}
	if output.Type != input.Type {
		t.Fatal("wrong version")
	}
	if !output.Destination.EqualTo(input.Destination) {
		t.Fatal("wrong path")
	}
	if !bytes.Equal(input.Payload, output.Payload) {
		fmt.Println("want: ", input.Payload)
		fmt.Println("got: ", output.Payload)
		t.Fatal("wrong payload")
	}
}

func TestUpdatePath(t *testing.T) {
	frame := Frame{
		Version:     Version0,
		Type:        TypeSource,
		Destination: SwitchPorts{1, 2, 3, 4},
		Source:      SwitchPorts{},
	}
	expectedDst := SwitchPorts{2, 3, 4}
	expectedSrc := SwitchPorts{5}
	frame.UpdateSourceRoutedPath(5)
	if !frame.Destination.EqualTo(expectedDst) {
		t.Fatalf("got %v, expected %v", frame.Destination, expectedDst)
	}
	if !frame.Source.EqualTo(expectedSrc) {
		t.Fatalf("got %v, expected %v", frame.Source, expectedSrc)
	}
}
