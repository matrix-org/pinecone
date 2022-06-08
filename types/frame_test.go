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

func TestMarshalUnmarshalSNEKBootstrapFrame(t *testing.T) {
	pk, _, _ := ed25519.GenerateKey(nil)
	wpk, _, _ := ed25519.GenerateKey(nil)
	input := Frame{
		Version: Version0,
		Type:    TypeVirtualSnakeBootstrap,
		Payload: []byte{9, 9, 9, 9, 9},
		Watermark: VirtualSnakeWatermark{
			Sequence: 100,
		},
	}
	copy(input.DestinationKey[:], pk)
	copy(input.Watermark.PublicKey[:], wpk)
	input.Watermark.Sequence = 100
	expected := []byte{
		0x70, 0x69, 0x6e, 0x65, // magic bytes
		0,                               // version 0
		byte(TypeVirtualSnakeBootstrap), // type greedy
		0, 0,                            // extra
		0, 82, // frame length
		0, 5, // payload length
	}
	expected = append(expected, pk...)
	expected = append(expected, wpk...)
	var seq [4]byte
	n, err := input.Watermark.Sequence.MarshalBinary(seq[:])
	if err != nil {
		t.Fatal(err)
	}
	expected = append(expected, seq[:n]...)
	expected = append(expected, input.Payload...)
	buf := make([]byte, 65535)
	n, err = input.MarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Got", buf[:n])
	fmt.Println("Want", expected)
	if n != len(expected) {
		t.Fatalf("wrong marshalled length, got %d, expected %d", n, len(expected))
	}
	if !bytes.Equal(buf[:n], expected) {
		t.Fatalf("wrong marshalled output, got %v", buf[:n])
	}

	t.Log("Got: ", buf[:n])
	t.Log("Want:", expected)

	output := Frame{
		Payload: make([]byte, 0, MaxPayloadSize),
	}
	if _, err := output.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if output.Version != input.Version {
		t.Fatal("wrong version")
	}
	if output.Type != input.Type {
		t.Fatal("wrong version")
	}
	if !bytes.Equal(input.Payload, output.Payload) {
		t.Fatal("wrong payload")
	}
}

func TestMarshalUnmarshalSNEKFrame(t *testing.T) {
	pk1, _, _ := ed25519.GenerateKey(nil)
	pk2, _, _ := ed25519.GenerateKey(nil)
	wpk, _, _ := ed25519.GenerateKey(nil)
	input := Frame{
		Version: Version0,
		Type:    TypeVirtualSnakeRouted,
		Payload: []byte("HELLO!"),
		Watermark: VirtualSnakeWatermark{
			Sequence: 100,
		},
	}
	copy(input.SourceKey[:], pk1)
	copy(input.DestinationKey[:], pk2)
	copy(input.Watermark.PublicKey[:], wpk)
	expected := []byte{
		0x70, 0x69, 0x6e, 0x65, // magic bytes
		0,                            // version 0
		byte(TypeVirtualSnakeRouted), // type greedy
		0, 0,                         // extra
		0, 115, // frame length
		0, 6, // payload length
	}
	expected = append(expected, pk2...)
	expected = append(expected, pk1...)
	expected = append(expected, wpk...)
	var seq [4]byte
	n, err := input.Watermark.Sequence.MarshalBinary(seq[:])
	if err != nil {
		t.Fatal(err)
	}
	expected = append(expected, seq[:n]...)
	expected = append(expected, input.Payload...)
	buf := make([]byte, 65535)
	n, err = input.MarshalBinary(buf)
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

	output := Frame{
		Payload: make([]byte, 0, MaxPayloadSize),
	}
	if _, err := output.UnmarshalBinary(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if output.Version != input.Version {
		t.Fatal("wrong version")
	}
	if output.Type != input.Type {
		t.Fatal("wrong version")
	}
	if !bytes.Equal(input.Payload, output.Payload) {
		fmt.Println("want: ", input.Payload)
		fmt.Println("got: ", output.Payload)
		t.Fatal("wrong payload")
	}
}
