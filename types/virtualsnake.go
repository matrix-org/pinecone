package types

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

const VirtualSnakePathIDLength = 8

type VirtualSnakePathID [VirtualSnakePathIDLength]byte
type VirtualSnakePathSig [ed25519.SignatureSize]byte

func (p VirtualSnakePathID) MarshalJSON() ([]byte, error) {
	return []byte("\"" + hex.EncodeToString(p[:]) + "\""), nil
}

func (a VirtualSnakePathID) CompareTo(b VirtualSnakePathID) int {
	return bytes.Compare(a[:], b[:])
}

type VirtualSnakeBootstrap struct {
	PathID    VirtualSnakePathID
	SourceSig VirtualSnakePathSig
	Root
}

func (v *VirtualSnakeBootstrap) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength+v.Root.Length()+ed25519.SignatureSize {
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
	offset += copy(buf[offset:], v.SourceSig[:])
	return offset, nil
}

func (v *VirtualSnakeBootstrap) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength+v.Root.MinLength()+ed25519.SignatureSize {
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
	offset += copy(v.SourceSig[:], buf[offset:])
	return offset, nil
}

type VirtualSnakeBootstrapACK struct {
	PathID         VirtualSnakePathID
	SourceSig      VirtualSnakePathSig
	DestinationSig VirtualSnakePathSig
	Root
}

func (v *VirtualSnakeBootstrapACK) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength+v.Root.Length()+(ed25519.SignatureSize*2) {
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
	offset += copy(buf[offset:], v.SourceSig[:])
	offset += copy(buf[offset:], v.DestinationSig[:])
	return offset, nil
}

func (v *VirtualSnakeBootstrapACK) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength+v.Root.MinLength()+(ed25519.SignatureSize*2) {
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
	offset += copy(v.SourceSig[:], buf[offset:])
	offset += copy(v.DestinationSig[:], buf[offset:])
	return offset, nil
}

type VirtualSnakeSetup struct {
	PathID         VirtualSnakePathID
	SourceSig      VirtualSnakePathSig
	DestinationSig VirtualSnakePathSig
	Root
}

func (v *VirtualSnakeSetup) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength+v.Root.Length()+(ed25519.SignatureSize*2) {
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
	offset += copy(buf[offset:], v.SourceSig[:])
	offset += copy(buf[offset:], v.DestinationSig[:])
	return offset, nil
}

func (v *VirtualSnakeSetup) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength+v.Root.MinLength()+(ed25519.SignatureSize*2) {
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
	offset += copy(v.SourceSig[:], buf[offset:])
	offset += copy(v.DestinationSig[:], buf[offset:])
	return offset, nil
}

type VirtualSnakeSetupACK struct {
	PathID    VirtualSnakePathID
	TargetSig VirtualSnakePathSig
	Root
}

func (v *VirtualSnakeSetupACK) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength+v.Root.Length()+ed25519.SignatureSize {
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
	offset += copy(buf[offset:], v.TargetSig[:])
	return offset, nil
}

func (v *VirtualSnakeSetupACK) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength+v.Root.MinLength()+ed25519.SignatureSize {
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
	offset += copy(v.TargetSig[:], buf[offset:])
	return offset, nil
}

type VirtualSnakeTeardown struct {
	PathID VirtualSnakePathID
}

func (v *VirtualSnakeTeardown) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	return offset, nil
}

func (v *VirtualSnakeTeardown) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < VirtualSnakePathIDLength {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	return offset, nil
}
