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
	input := Varu64(12345678)
	expected := []byte{133, 241, 194, 78}
	bin, err := input.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bin, expected) {
		t.Fatalf("expected %v, got %v", expected, bin)
	}
	if length := input.Length(); length != len(expected) {
		t.Fatalf("expected length %d, got %d", length, len(expected))
	}
}

func TestUnmarshalBinaryVaru64(t *testing.T) {
	input := []byte{133, 241, 194, 78, 0, 1, 2, 3, 4, 5, 6}
	expected := Varu64(12345678)
	var num Varu64
	if err := num.UnmarshalBinary(input); err != nil {
		t.Fatal(err)
	}
	if num != expected {
		t.Fatalf("expected %v, got %v", expected, num)
	}
}
