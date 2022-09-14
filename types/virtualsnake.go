package types

import (
	"crypto/ed25519"
	"fmt"
)

type VirtualSnakeBootstrap struct {
	Sequence Varu64
	Root
	Signature [ed25519.SignatureSize]byte
}

type VirtualSnakeWatermark struct {
	PublicKey PublicKey `json:"public_key"`
	Sequence  Varu64    `json:"sequence"`
}

func (a VirtualSnakeWatermark) WorseThan(b VirtualSnakeWatermark) bool {
	diff := a.PublicKey.CompareTo(b.PublicKey)
	return diff > 0 || (diff == 0 && a.Sequence != 0 && a.Sequence < b.Sequence)
}

func (v *VirtualSnakeBootstrap) ProtectedPayload() ([]byte, error) {
	buffer := make([]byte, v.Sequence.Length()+v.Root.Length())
	offset := 0
	n, err := v.Sequence.MarshalBinary(buffer[:])
	if err != nil {
		return nil, fmt.Errorf("v.Sequence.MarshalBinary: %w", err)
	}
	offset += n
	offset += copy(buffer[offset:], v.RootPublicKey[:])
	n, err = v.RootSequence.MarshalBinary(buffer[offset:])
	if err != nil {
		return nil, fmt.Errorf("v.RootSequence.MarshalBinary: %w", err)
	}
	offset += n
	return buffer[:offset], nil
}

func (v *VirtualSnakeBootstrap) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < v.Sequence.Length()+v.Root.Length()+ed25519.SignatureSize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	n, err := v.Sequence.MarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.Sequence.MarshalBinary: %w", err)
	}
	offset += n
	offset += copy(buf[offset:], v.RootPublicKey[:])
	n, err = v.RootSequence.MarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.RootSequence.MarshalBinary: %w", err)
	}
	offset += n
	offset += copy(buf[offset:], v.Signature[:])
	return offset, nil
}

func (v *VirtualSnakeBootstrap) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < v.Sequence.MinLength()+v.Root.MinLength()+ed25519.SignatureSize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	n, err := v.Sequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.Sequence.UnmarshalBinary: %w", err)
	}
	offset += n
	offset += copy(v.RootPublicKey[:], buf[offset:])
	n, err = v.RootSequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, fmt.Errorf("v.RootSequence.UnmarshalBinary: %w", err)
	}
	offset += n
	offset += copy(v.Signature[:], buf[offset:])
	return offset, nil
}
