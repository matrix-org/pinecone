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

func TestMarshalBinaryVaru64(t *testing.T) {
	var bin [4]byte
	for input, expected := range map[Varu64][]byte{
		0:        {0},
		1:        {1},
		12:       {12},
		123:      {123},
		1234:     {137, 82},
		12345:    {224, 57},
		123456:   {135, 196, 64},
		1234567:  {203, 173, 7},
		12345678: {133, 241, 194, 78},
	} {
		n, err := input.MarshalBinary(bin[:])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bin[:n], expected) {
			t.Fatalf("for %d expected %v, got %v", input, expected, bin[:n])
		}
		if length := input.Length(); length != len(expected) {
			t.Fatalf("for %d expected length %d, got %d", input, length, len(expected))
		}
	}
}

func TestUnmarshalBinaryVaru64(t *testing.T) {
	for input, expected := range map[[7]byte]Varu64{
		{0}:                          0,
		{1}:                          1,
		{12}:                         12,
		{123}:                        123,
		{137, 82}:                    1234,
		{224, 57}:                    12345,
		{135, 196, 64}:               123456,
		{203, 173, 7}:                1234567,
		{133, 241, 194, 78}:          12345678,
		{133, 241, 194, 78, 1, 2, 3}: 12345678,
	} {
		var num Varu64
		if _, err := num.UnmarshalBinary(input[:]); err != nil {
			t.Fatal(err)
		}
		if num != expected {
			t.Fatalf("expected %v, got %v", expected, num)
		}
	}
}
