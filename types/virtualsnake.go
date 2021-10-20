package types

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

type VirtualSnakePathID [8]byte

func (p VirtualSnakePathID) MarshalJSON() ([]byte, error) {
	return []byte("\"" + hex.EncodeToString(p[:]) + "\""), nil
}

type VirtualSnakeBootstrap struct {
	PathID VirtualSnakePathID
	Root
}

func (v *VirtualSnakeBootstrap) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	offset += copy(buf[offset:], v.RootPublicKey[:])
	n, err := v.RootSequence.MarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.RootSequence.MarshalBinary: %w", err)
	}
	offset += n
	return offset, nil
}

func (v *VirtualSnakeBootstrap) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	offset += copy(v.RootPublicKey[:], buf[offset:])
	l, err := v.RootSequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.RootSequence.UnmarshalBinary: %w", err)
	}
	offset += l
	return offset, nil
}

type VirtualSnakeBootstrapACK struct {
	PathID VirtualSnakePathID
	Root
}

func (v *VirtualSnakeBootstrapACK) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize+v.RootSequence.Length() {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	offset += copy(buf[offset:], v.RootPublicKey[:])
	n, err := v.RootSequence.MarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.RootSequence.MarshalBinary: %w", err)
	}
	offset += n
	return offset, nil
}

func (v *VirtualSnakeBootstrapACK) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize+1 {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	offset += copy(v.RootPublicKey[:], buf[offset:])
	l, err := v.RootSequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.RootSequence.UnmarshalBinary: %w", err)
	}
	offset += l
	return offset, nil
}

type VirtualSnakeSetup struct {
	PathID VirtualSnakePathID
	Root
}

func (v *VirtualSnakeSetup) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize+v.RootSequence.Length() {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	offset += copy(buf[offset:], v.RootPublicKey[:])
	n, err := v.RootSequence.MarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.RootSequence.MarshalBinary: %w", err)
	}
	offset += n
	return offset, nil
}

func (v *VirtualSnakeSetup) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize+1 {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	offset += copy(v.RootPublicKey[:], buf[offset:])
	l, err := v.RootSequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.RootSequence.UnmarshalBinary: %w", err)
	}
	offset += l
	return offset, nil
}

type VirtualSnakeTeardown struct {
	PathID VirtualSnakePathID
}

func (v *VirtualSnakeTeardown) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8 {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	return offset, nil
}

func (v *VirtualSnakeTeardown) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8 {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	return offset, nil
}
