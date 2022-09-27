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

//go:build darwin
// +build darwin

package multicast

import "C"

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func (m *Multicast) multicastStarted() { // nolint:unused

}

func (m *Multicast) udpOptions(network string, address string, c syscall.RawConn) error {
	var reuseport error
	//var recvanyif error
	control := c.Control(func(fd uintptr) {
		reuseport = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		//	recvanyif = unix.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 0x1104, 1) // SO_RECV_ANYIF
	})

	switch {
	case reuseport != nil:
		return fmt.Errorf("SO_REUSEPORT: %w", reuseport)
	//case recvanyif != nil:
	//	return fmt.Errorf("SO_RECV_ANYIF: %w", recvanyif)
	default:
		return control
	}
}

func (m *Multicast) tcpOptions(network string, address string, c syscall.RawConn) error {
	/*
		var recvanyif error
		control := c.Control(func(fd uintptr) {
			recvanyif = unix.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 0x1104, 1) // SO_RECV_ANYIF
		})

		switch {
		case recvanyif != nil:
			return fmt.Errorf("SO_RECV_ANYIF: %w", recvanyif)
		default:
			return control
		}
	*/
	return nil
}
