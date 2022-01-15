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
)

// MaxPayloadSize is the maximum size that a single frame can contain
// as a payload, not including headers.
const MaxPayloadSize = 65535

// MaxFrameSize is the maximum size that a single frame can be, including
// all headers.
const MaxFrameSize = 65535*3 + 16

type FrameVersion uint8
type FrameType uint8

const (
	TypeKeepalive                FrameType = iota       // protocol frame, direct to peers only
	TypeTreeAnnouncement                                // protocol frame, bypasses queues
	TypeTreeRouted                                      // traffic frame, forwarded using tree routing
	TypeVirtualSnakeBootstrap                           // protocol frame, forwarded using SNEK
	TypeVirtualSnakeBootstrapACK                        // protocol frame, forwarded using tree routing
	TypeVirtualSnakeSetup                               // protocol frame, forwarded using tree routing
	TypeVirtualSnakeSetupACK                            // protocol frame, forwarded using special rules
	TypeVirtualSnakeTeardown                            // protocol frame, forwarded using special rules
	TypeVirtualSnakeRouted                              // traffic frame, forwarded using SNEK
	TypeSNEKPing                 FrameType = iota + 200 // traffic frame, forwarded using SNEK
	TypeSNEKPong                                        // traffic frame, forwarded using SNEK
	TypeTreePing                                        // traffic frame, forwarded using tree
	TypeTreePong                                        // traffic frame, forwarded using tree
)

const (
	Version0 FrameVersion = iota
)

var FrameMagicBytes = []byte{0x70, 0x69, 0x6e, 0x65}

// 4 magic bytes, 1 byte version, 1 byte type, 2 bytes extra, 2 bytes frame length
const FrameHeaderLength = 10

type Frame struct {
	Version        FrameVersion
	Type           FrameType
	Extra          [2]byte
	Destination    Coordinates
	DestinationKey PublicKey
	Source         Coordinates
	SourceKey      PublicKey
	Payload        []byte
}

func (f *Frame) Reset() {
	f.Version, f.Type = 0, 0
	for i := range f.Extra {
		f.Extra[i] = 0
	}
	f.Destination = Coordinates{}
	f.DestinationKey = PublicKey{}
	f.Source = Coordinates{}
	f.SourceKey = PublicKey{}
	f.Payload = f.Payload[:0]
}

func (f *Frame) MarshalBinary(buffer []byte) (int, error) {
	copy(buffer[:4], FrameMagicBytes)
	buffer[4], buffer[5] = byte(f.Version), byte(f.Type)
	copy(buffer[6:], f.Extra[:])
	offset := FrameHeaderLength
	switch f.Type {
	case TypeVirtualSnakeBootstrap: // destination = key, source = coords
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		offset += 2
		n, err := f.Source.MarshalBinary(buffer[offset:])
		if err != nil {
			return 0, fmt.Errorf("f.Source.MarshalBinary: %w", err)
		}
		offset += n
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			f.Payload = f.Payload[:payloadLen]
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnakeBootstrapACK:
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		dn, err := f.Destination.MarshalBinary(buffer[offset+6:])
		if err != nil {
			return 0, fmt.Errorf("f.Destination.MarshalBinary: %w", err)
		}
		sn, err := f.Source.MarshalBinary(buffer[offset+6+dn:])
		if err != nil {
			return 0, fmt.Errorf("f.Source.MarshalBinary: %w", err)
		}
		binary.BigEndian.PutUint16(buffer[offset+2:offset+4], uint16(dn))
		binary.BigEndian.PutUint16(buffer[offset+4:offset+6], uint16(sn))
		offset += 6 + dn + sn
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		offset += copy(buffer[offset:], f.SourceKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			f.Payload = f.Payload[:payloadLen]
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnakeSetup: // destination = coords & key, source = key
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		dn, err := f.Destination.MarshalBinary(buffer[offset+2:])
		if err != nil {
			return 0, fmt.Errorf("f.Destination.MarshalBinary: %w", err)
		}
		offset += 2 + dn
		offset += copy(buffer[offset:], f.SourceKey[:ed25519.PublicKeySize])
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			f.Payload = f.Payload[:payloadLen]
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnakeSetupACK: // detination = key
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		offset += 2
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			f.Payload = f.Payload[:payloadLen]
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnakeTeardown: // destination = key
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		offset += 2
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			f.Payload = f.Payload[:payloadLen]
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeVirtualSnakeRouted, TypeSNEKPing, TypeSNEKPong: // destination = key, source = key
		payloadLen := len(f.Payload)
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(payloadLen))
		offset += 2
		offset += copy(buffer[offset:], f.DestinationKey[:ed25519.PublicKeySize])
		offset += copy(buffer[offset:], f.SourceKey[:ed25519.PublicKeySize])
		if f.Payload != nil {
			f.Payload = f.Payload[:payloadLen]
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}

	case TypeKeepalive:

	default: // destination = coords, source = coords
		dn, err := f.Destination.MarshalBinary(buffer[offset+6:])
		if err != nil {
			return 0, fmt.Errorf("f.Destination.MarshalBinary: %w", err)
		}
		sn, err := f.Source.MarshalBinary(buffer[offset+6+dn:])
		if err != nil {
			return 0, fmt.Errorf("f.Source.MarshalBinary: %w", err)
		}
		payloadLen := len(f.Payload)
		if dn > math.MaxUint16 || sn > math.MaxUint16 || payloadLen > math.MaxUint16 {
			return 0, fmt.Errorf("frame contents too large")
		}
		binary.BigEndian.PutUint16(buffer[offset+0:offset+2], uint16(dn))
		binary.BigEndian.PutUint16(buffer[offset+2:offset+4], uint16(sn))
		binary.BigEndian.PutUint16(buffer[offset+4:offset+6], uint16(payloadLen))
		offset += 6 + dn + sn
		if f.Payload != nil {
			f.Payload = f.Payload[:payloadLen]
			offset += copy(buffer[offset:], f.Payload[:payloadLen])
		}
	}

	binary.BigEndian.PutUint16(buffer[FrameHeaderLength-2:FrameHeaderLength], uint16(offset))
	return offset, nil
}

func (f *Frame) UnmarshalBinary(data []byte) (int, error) {
	f.Reset()
	if len(data) < FrameHeaderLength {
		return 0, fmt.Errorf("frame is not long enough to include metadata")
	}
	if !bytes.Equal(data[:4], FrameMagicBytes) {
		return 0, fmt.Errorf("frame doesn't contain magic bytes")
	}
	f.Version, f.Type = FrameVersion(data[4]), FrameType(data[5])
	f.Extra[0], f.Extra[1] = data[6], data[7]
	copy(f.Extra[:], data[6:])
	framelen := int(binary.BigEndian.Uint16(data[FrameHeaderLength-2 : FrameHeaderLength]))
	if len(data) != framelen {
		return 0, fmt.Errorf("frame length incorrect")
	}
	offset := FrameHeaderLength
	switch f.Type {
	case TypeVirtualSnakeBootstrap: // destination = key, source = coords
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		if payloadLen > cap(f.Payload) {
			return 0, fmt.Errorf("payload length exceeds frame capacity")
		}
		srcLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if _, err := f.Source.UnmarshalBinary(data[offset+2:]); err != nil {
			return 0, fmt.Errorf("f.Source.UnmarshalBinary: %w", err)
		}
		offset += 4 + srcLen
		offset += copy(f.DestinationKey[:], data[offset:])
		f.Payload = f.Payload[:payloadLen]
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnakeBootstrapACK:
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		if payloadLen > cap(f.Payload) {
			return 0, fmt.Errorf("payload length exceeds frame capacity")
		}
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
		f.Payload = f.Payload[:payloadLen]
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnakeSetup: // destination = coords & key, source = key
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		if payloadLen > cap(f.Payload) {
			return 0, fmt.Errorf("payload length exceeds frame capacity")
		}
		dstLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if _, err := f.Destination.UnmarshalBinary(data[offset+2:]); err != nil {
			return 0, fmt.Errorf("f.Destination.UnmarshalBinary: %w", err)
		}
		offset += 4 + dstLen
		offset += copy(f.SourceKey[:], data[offset:])
		offset += copy(f.DestinationKey[:], data[offset:])
		f.Payload = f.Payload[:payloadLen]
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnakeSetupACK: // destination = key
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		if payloadLen > cap(f.Payload) {
			return 0, fmt.Errorf("payload length exceeds frame capacity")
		}
		offset += 2
		offset += copy(f.DestinationKey[:], data[offset:])
		f.Payload = f.Payload[:payloadLen]
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnakeTeardown: // destination = key
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		if payloadLen > cap(f.Payload) {
			return 0, fmt.Errorf("payload length exceeds frame capacity")
		}
		offset += 2
		offset += copy(f.DestinationKey[:], data[offset:])
		f.Payload = f.Payload[:payloadLen]
		offset += copy(f.Payload, data[offset:])
		return offset, nil

	case TypeVirtualSnakeRouted, TypeSNEKPing, TypeSNEKPong: // destination = key, source = key
		payloadLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		if payloadLen > cap(f.Payload) {
			return 0, fmt.Errorf("payload length exceeds frame capacity")
		}
		offset += 2
		offset += copy(f.DestinationKey[:], data[offset:])
		offset += copy(f.SourceKey[:], data[offset:])
		f.Payload = f.Payload[:payloadLen]
		offset += copy(f.Payload, data[offset:])
		return offset + payloadLen, nil

	case TypeKeepalive:
		return offset, nil

	default: // destination = coords, source = coords
		dstLen := int(binary.BigEndian.Uint16(data[offset+0 : offset+2]))
		srcLen := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		payloadLen := int(binary.BigEndian.Uint16(data[offset+4 : offset+6]))
		if payloadLen > cap(f.Payload) {
			return 0, fmt.Errorf("payload length exceeds frame capacity")
		}
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
		f.Payload = f.Payload[:payloadLen]
		offset += copy(f.Payload, data[offset:])
		return offset + payloadLen, nil
	}
}

func (t FrameType) String() string {
	switch t {
	case TypeTreeAnnouncement:
		return "TreeAnnouncement"
	case TypeTreeRouted:
		return "TreeRouted"
	case TypeVirtualSnakeBootstrap:
		return "VirtualSnakeBootstrap"
	case TypeVirtualSnakeBootstrapACK:
		return "VirtualSnakeBootstrapACK"
	case TypeVirtualSnakeSetup:
		return "VirtualSnakeSetup"
	case TypeVirtualSnakeSetupACK:
		return "VirtualSnakeSetupACK"
	case TypeVirtualSnakeRouted:
		return "VirtualSnakeRouted"
	case TypeVirtualSnakeTeardown:
		return "VirtualSnakeTeardown"
	case TypeKeepalive:
		return "Keepalive"
	case TypeSNEKPing:
		return "SNEKPing"
	case TypeSNEKPong:
		return "SNEKPong"
	case TypeTreePing:
		return "TreePing"
	case TypeTreePong:
		return "TreePong"
	default:
		return "Unknown"
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
