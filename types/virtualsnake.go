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
	Sequence Varu64
	Root
	Signatures []VirtualSnakeBootstrapSignature
}

type VirtualSnakeBootstrapSignature struct {
	Signature [ed25519.SignatureSize]byte `json:"-"`
	PublicKey PublicKey                   `json:"public_key"`
}

type VirtualSnakeWatermark struct {
	PublicKey PublicKey `json:"public_key"`
	Sequence  Varu64    `json:"sequence"`
}

func (a VirtualSnakeWatermark) WorseThan(b VirtualSnakeWatermark) bool {
	diff := a.PublicKey.CompareTo(b.PublicKey)
	return diff > 0 || (diff == 0 && a.Sequence < b.Sequence)
}

func (v *VirtualSnakeBootstrap) Sign(private PrivateKey) error {
	// TODO: This probably creates quite a lot of allocations
	// so we should do something about that.
	edprivate := append(ed25519.PrivateKey{}, private[:]...)
	var buffer [65535]byte
	n, err := v.MarshalBinary(buffer[:])
	if err != nil {
		return fmt.Errorf("v.MarshalBinary: %w", err)
	}
	sig := VirtualSnakeBootstrapSignature{}
	copy(sig.PublicKey[:], edprivate.Public().(ed25519.PublicKey))
	copy(sig.Signature[:], ed25519.Sign(private[:], buffer[:n]))
	v.Signatures = append(v.Signatures, sig)
	return nil
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
	for _, sig := range v.Signatures {
		n, err := sig.MarshalBinary(buf[offset:])
		if err != nil {
			return 0, fmt.Errorf("sig.MarshalBinary: %w", err)
		}
		offset += n
	}
	return offset, nil
}

func (v *VirtualSnakeBootstrap) UnmarshalBinary(buf []byte) (int, int, error) {
	if len(buf) < v.Sequence.MinLength()+v.Root.MinLength()+ed25519.SignatureSize {
		return 0, 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	n, err := v.Sequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, 0, fmt.Errorf("v.Sequence.UnmarshalBinary: %w", err)
	}
	offset += n
	offset += copy(v.RootPublicKey[:], buf[offset:])
	n, err = v.RootSequence.UnmarshalBinary(buf[offset:])
	if err != nil {
		return 0, 0, fmt.Errorf("v.RootSequence.UnmarshalBinary: %w", err)
	}
	offset += n
	sigoffset := offset
	for offset < len(buf) {
		var sig VirtualSnakeBootstrapSignature
		n, err := sig.UnmarshalBinary(buf[offset:])
		if err != nil {
			return 0, 0, fmt.Errorf("sig.MarshalBinary: %w", err)
		}
		v.Signatures = append(v.Signatures, sig)
		offset += n
	}
	return offset, sigoffset, nil
}

func (v *VirtualSnakeBootstrapSignature) MarshalBinary(buf []byte) (int, error) {
	offset := 0
	offset += copy(buf[offset:], v.PublicKey[:])
	offset += copy(buf[offset:], v.Signature[:])
	return offset, nil
}

func (v *VirtualSnakeBootstrapSignature) UnmarshalBinary(buf []byte) (int, error) {
	offset := 0
	offset += copy(v.PublicKey[:], buf[offset:])
	offset += copy(v.Signature[:], buf[offset:])
	return offset, nil
}
