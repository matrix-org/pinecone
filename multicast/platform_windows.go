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

// +build windows

package multicast

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

func (m *Multicast) multicastStarted() {
}

func (m *Multicast) udpOptions(network string, address string, c syscall.RawConn) error {
	var reuseport error
	control := c.Control(func(fd uintptr) {
		reuseport = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
	})

	switch {
	case reuseport != nil:
		return fmt.Errorf("SO_REUSEPORT: %w", reuseport)
	default:
		return control
	}
}

func (m *Multicast) tcpOptions(network string, address string, c syscall.RawConn) error {
	return nil
}
