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
	"encoding/binary"
	"fmt"
	"strings"
)

type SwitchPortID Varu64
type SwitchPorts []SwitchPortID

func (s SwitchPorts) Len() int {
	return len(s)
}

func (s SwitchPorts) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s SwitchPorts) Less(i, j int) bool {
	return s[i] < s[j]
}

func (s SwitchPorts) String() string {
	ports := make([]string, 0, len(s))
	for _, p := range s {
		ports = append(ports, fmt.Sprintf("%d", p))
	}
	return "[" + strings.Join(ports, " ") + "]"
}

func (p SwitchPorts) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 2, 2+len(p)*10)
	l := 0
	for _, a := range p {
		b, err := Varu64(a).MarshalBinary()
		if err != nil {
			return buf, fmt.Errorf("Varu64(a).MarshalBinary: %w", err)
		}
		buf = append(buf, b...)
		l += len(b)
	}
	binary.BigEndian.PutUint16(buf[:2], uint16(l))
	return buf[:l+2], nil
}

func (p *SwitchPorts) UnmarshalBinary(b []byte) (int, error) {
	l := int(binary.BigEndian.Uint16(b[:2]))
	ports := make(SwitchPorts, 0, 16)
	if rl := len(b); rl < 2+l {
		return 0, fmt.Errorf("expecting %d bytes but got %d bytes", 2+l, rl)
	}
	read := 2
	b = b[read : 2+l]
	for {
		if len(b) < 1 {
			break
		}
		var id Varu64
		if err := id.UnmarshalBinary(b); err != nil {
			return 0, fmt.Errorf("id.UnmarshalBinary: %w", err)
		}
		ports = append(ports, SwitchPortID(id))
		b = b[id.Length():]
		read += id.Length()
	}
	*p = ports
	return read, nil
}

func (p SwitchPorts) EqualTo(o SwitchPorts) bool {
	if len(p) != len(o) {
		return false
	}
	for i := range p {
		if p[i] != o[i] {
			return false
		}
	}
	return true
}

func (a *SwitchPorts) Copy() SwitchPorts {
	return append(SwitchPorts{}, *a...)
}

func (a SwitchPorts) DistanceTo(b SwitchPorts) int {
	ancestor := getCommonPrefix(a, b)
	return len(a) + len(b) - 2*ancestor
}

func getCommonPrefix(a, b SwitchPorts) int {
	c := -1
	l := len(a)
	if len(b) < l {
		l = len(b)
	}
	for i := 0; i < l; i++ {
		if a[i] != b[i] {
			break
		}
		c = i
	}
	return c
}
