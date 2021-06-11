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
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"sync"

	"go.uber.org/atomic"
)

var framePool = &sync.Pool{
	New: func() interface{} {
		f := &Frame{}
		runtime.SetFinalizer(f, func(f *Frame) {
			if refs := f.refs.Load(); refs != 0 {
				panic(fmt.Sprintf("frame was garbage collected with %d remaining references, this is a bug", refs))
			}
		})
		return f
	},
}

func GetFrame() *Frame {
	frame := framePool.Get().(*Frame)
	frame.refs.Store(1)
	return frame
}

func (f *Frame) Borrow() *Frame {
	f.refs.Inc()
	return f
}

func (f *Frame) Done() bool {
	refs := f.refs.Dec()
	if refs < 0 {
		panic("invalid Done call")
	}
	if refs == 0 {
		f.Reset()
		framePool.Put(f)
		return true
	}
	return false
}

type FrameVersion uint8
type FrameType uint8

const (
	TypeSTP                      FrameType = iota // protocol frame, bypasses queues
	TypeSource                                    // traffic frame, forwarded using source routing
	TypeGreedy                                    // traffic frame, forwarded using greedy routing
	TypePathfind                                  // protocol frame, sign the update before forwarding it greedily
	TypeSwitchUpdate                              // sent from the switch to the router to update about peers
	TypeDHTRequest                                // protocol frame, forwarded using greedy routing
	TypeDHTResponse                               // protocol frame, forwarded using greedy routing
	TypeVirtualSnakeBootstrap                     // protocol frame, forwarded using SNEK predecessor ordering
	TypeVirtualSnakeBootstrapACK                  // protocol frame, forwarded using greedy routing
	TypeVirtualSnakeSetup                         // protocol frame, forwarded using greedy routing
	TypeVirtualSnake                              // traffic frame, forwarded using SNEK successor ordering
	TypeVirtualSnakePathfind                      // protocol frame, forwarded using SNEK successor ordering
	TypeVirtualSnakeTeardown                      // protocol frame
	TypeKeepalive                                 // protocol frame, bypasses queues
)

const (
	Version0 FrameVersion = iota
)

var FrameMagicBytes = []byte{0x70, 0x69, 0x6e, 0x65}

type Frame struct {
	refs           atomic.Int32
	Version        FrameVersion
	Type           FrameType
	Destination    SwitchPorts
	DestinationKey PublicKey
	Source         SwitchPorts
	SourceKey      PublicKey
	Payload        []byte
}

func (f *Frame) Reset() {
	f.Version, f.Type = 0, 0
	f.Destination = SwitchPorts{}
	f.DestinationKey = PublicKey{}
	f.Source = SwitchPorts{}
	f.SourceKey = PublicKey{}
	f.Payload = f.Payload[:0]
}

func (f *Frame) Copy() *Frame {
	copy := *f
	copy.refs.Store(1)
	copy.Payload = append([]byte{}, f.Payload...)
	return &copy
}

func (f *Frame) MarshalBinary(buffer []byte) (int, error) {
	copy(buffer[:4], FrameMagicBytes)
	buffer[4], buffer[5] = byte(f.Version), byte(f.Type)
	offset := 8
	switch f.Type {
	case TypeVirtualSnakeBootstrap: // destination = key, source = coords
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		src, err := f.Source.MarshalBinary()
		if err != nil {
			return 0, fmt.Errorf("f.Source.MarshalBinary: %w", err)
		}
		offset += 2
		offset += copy(buffer[offset:], src)
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnakeBootstrapACK:
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		dst, err := f.Destination.MarshalBinary()
		if err != nil {
			return 0, fmt.Errorf("f.Destination.MarshalBinary: %w", err)
		}
		src, err := f.Source.MarshalBinary()
		if err != nil {
			return 0, fmt.Errorf("f.Source.MarshalBinary: %w", err)
		}
		binary.BigEndian.PutUint16(buffer[offset+2:offset+4], uint16(len(dst)))
		binary.BigEndian.PutUint16(buffer[offset+4:offset+6], uint16(len(src)))
		offset += 6
		offset += copy(buffer[offset:], dst)
		offset += copy(buffer[offset:], src)
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		offset += copy(buffer[offset:], f.SourceKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnakeSetup: // destination = coords & key, source = key
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		dst, err := f.Destination.MarshalBinary()
		if err != nil {
			return 0, fmt.Errorf("f.Destination.MarshalBinary: %w", err)
		}
		offset += 2
		offset += copy(buffer[offset:], dst)
		offset += copy(buffer[offset:], f.SourceKey[:ed25519.PublicKeySize])
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnakeTeardown: // destination = key
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		offset += 2
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnake, TypeVirtualSnakePathfind: // destination = key, source = key
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		offset += 2
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		offset += copy(buffer[offset:], f.SourceKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeKeepalive:

	default: // destination = coords, source = coords
		dst, err := f.Destination.MarshalBinary()
		if err != nil {
			return 0, fmt.Errorf("f.Destination.MarshalBinary: %w", err)
		}
		src, err := f.Source.MarshalBinary()
		if err != nil {
			return 0, fmt.Errorf("f.Source.MarshalBinary: %w", err)
		}
		dstLen, srcLen, payloadLen := len(dst), len(src), len(f.Payload)
		if dstLen > math.MaxUint16 || srcLen > math.MaxUint16 || payloadLen > math.MaxUint16 {
			return 0, fmt.Errorf("frame contents too large")
		}
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(dstLen))
		binary.BigEndian.PutUint16(buffer[offset+2:offset+4], uint16(srcLen))
		binary.BigEndian.PutUint16(buffer[offset+4:offset+6], uint16(payloadLen))
		offset += 6
		offset += copy(buffer[offset:], dst)
		offset += copy(buffer[offset:], src)
		if f.Payload != nil {
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}
	}

	binary.BigEndian.PutUint16(buffer[6:8], uint16(offset))
	return offset, nil
}

func (f *Frame) UnmarshalBinary(data []byte) (int, error) {
	f.Reset()
	if len(data) < 8 {
		return 0, fmt.Errorf("frame is not long enough to include metadata")
	}
	if !bytes.Equal(data[:4], FrameMagicBytes) {
		return 0, fmt.Errorf("frame doesn't contain magic bytes")
	}
	f.Version, f.Type = FrameVersion(data[4]), FrameType(data[5])
	framelen := int(binary.BigEndian.Uint16(data[6:8]))
	if len(data) != framelen {
		return 0, fmt.Errorf("frame length incorrect")
	}
	offset := 8
	switch f.Type {
	case TypeVirtualSnakeBootstrap: // destination = key, source = coords
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		srcLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if _, err := f.Source.UnmarshalBinary(data[offset+2:]); err != nil {
			return 0, fmt.Errorf("f.Source.UnmarshalBinary: %w", err)
		}
		offset += 4 + srcLen
		offset += copy(f.DestinationKey[:], data[offset:])
		f.Payload = make([]byte, payloadLen)
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnakeBootstrapACK:
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		dstLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		srcLen := int(binary.BigEndian.Uint16(data[offset+4 : offset+6]))
		offset += 6
		if _, err := f.Destination.UnmarshalBinary(data[offset:]); err != nil {
			return 0, fmt.Errorf("f.Destination.UnmarshalBinary: %w", err)
		}
		offset += dstLen
		if _, err := f.Source.UnmarshalBinary(data[offset:]); err != nil {
			return 0, fmt.Errorf("f.Destination.UnmarshalBinary: %w", err)
		}
		offset += srcLen
		offset += copy(f.DestinationKey[:], data[offset:])
		offset += copy(f.SourceKey[:], data[offset:])
		f.Payload = make([]byte, payloadLen)
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnakeSetup: // destination = coords & key, source = key
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		dstLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if _, err := f.Destination.UnmarshalBinary(data[offset+2:]); err != nil {
			return 0, fmt.Errorf("f.Destination.UnmarshalBinary: %w", err)
		}
		offset += 4 + dstLen
		offset += copy(f.SourceKey[:], data[offset:])
		offset += copy(f.DestinationKey[:], data[offset:])
		f.Payload = make([]byte, payloadLen)
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnakeTeardown: // destination = key
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		offset += 2
		offset += copy(f.DestinationKey[:], data[offset:])
		f.Payload = make([]byte, payloadLen)
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnake, TypeVirtualSnakePathfind: // destination = key, source = key
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		offset += 2
		offset += copy(f.DestinationKey[:], data[offset:])
		offset += copy(f.SourceKey[:], data[offset:])
		f.Payload = make([]byte, payloadLen)
		offset += copy(f.Payload, data[offset:])
		return offset + payloadLen, nil

	case TypeKeepalive:
		return offset, nil

	default: // destination = coords, source = coords
		dstLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		srcLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		payloadLen := int(binary.BigEndian.Uint16(data[offset+4 : offset+6]))
		offset += 6
		if size := offset + dstLen + srcLen + payloadLen; len(data) != int(size) {
			return 0, fmt.Errorf("frame expecting %d total bytes, got %d bytes", size, len(data))
		}
		if _, err := f.Destination.UnmarshalBinary(data[offset : offset+dstLen]); err != nil {
			return 0, fmt.Errorf("f.Destination.UnmarshalBinary: %w", err)
		}
		offset += dstLen
		if _, err := f.Source.UnmarshalBinary(data[offset : offset+srcLen]); err != nil {
			return 0, fmt.Errorf("f.Source.UnmarshalBinary: %w", err)
		}
		offset += srcLen
		f.Payload = make([]byte, payloadLen)
		offset += copy(f.Payload, data[offset:])
		return offset + payloadLen, nil
	}
}

func (f *Frame) UpdateSourceRoutedPath(from SwitchPortID) {
	switch f.Type {
	case TypeSource:
		if len(f.Destination) > 0 {
			f.Destination = f.Destination[1:]
		}
		f.Source = append(SwitchPorts{from}, f.Source...)
	case TypeSTP:
		f.Destination = SwitchPorts{}
		f.Source = SwitchPorts{}
	default:
		panic("UpdateSourceRoutedPath: incorrect frame type")
	}
}

func (t FrameType) String() string {
	switch t {
	case TypeSTP:
		return "TypeSTP"
	case TypeSource:
		return "TypeSource"
	case TypeGreedy:
		return "TypeGreedy"
	case TypePathfind:
		return "TypePathfind"
	case TypeSwitchUpdate:
		return "TypeSwitchUpdate"
	case TypeDHTRequest:
		return "TypeDHTRequest"
	case TypeDHTResponse:
		return "TypeDHTResponse"
	case TypeVirtualSnakeBootstrap:
		return "TypeVirtualSnakeBootstrap"
	case TypeVirtualSnakeBootstrapACK:
		return "TypeVirtualSnakeBootstrapACK"
	case TypeVirtualSnakeSetup:
		return "TypeVirtualSnakeSetup"
	case TypeVirtualSnake:
		return "TypeVirtualSnake"
	case TypeVirtualSnakePathfind:
		return "TypeVirtualSnakePathfind"
	case TypeVirtualSnakeTeardown:
		return "TypeVirtualSnakeTeardown"
	default:
		return "TypeUnknown"
	}
}

func (v FrameVersion) String() string {
	switch v {
	case Version0:
		return "Version0"
	default:
		return "VersionUnknown"
	}
}
