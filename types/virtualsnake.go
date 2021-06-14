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
	PathID        VirtualSnakePathID
	RootPublicKey PublicKey
}

func (v *VirtualSnakeBootstrap) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	offset += copy(buf[offset:], v.RootPublicKey[:])
	return offset, nil
}

func (v *VirtualSnakeBootstrap) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	offset += copy(v.RootPublicKey[:], buf[offset:])
	return offset, nil
}

type VirtualSnakeBootstrapACK struct {
	PathID        VirtualSnakePathID
	RootPublicKey PublicKey
}

func (v *VirtualSnakeBootstrapACK) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	offset += copy(buf[offset:], v.RootPublicKey[:])
	return offset, nil
}

func (v *VirtualSnakeBootstrapACK) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	offset += copy(v.RootPublicKey[:], buf[offset:])
	return offset, nil
}

type VirtualSnakeSetup struct {
	PathID        VirtualSnakePathID
	RootPublicKey PublicKey
}

func (v *VirtualSnakeSetup) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	offset += copy(buf[offset:], v.RootPublicKey[:])
	return offset, nil
}

func (v *VirtualSnakeSetup) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < 8+ed25519.PublicKeySize {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	offset += copy(v.RootPublicKey[:], buf[offset:])
	return offset, nil
}

type VirtualSnakeTeardown struct {
	PathID    VirtualSnakePathID
	Ascending bool
}

func (v *VirtualSnakeTeardown) MarshalBinary(buf []byte) (int, error) {
	if len(buf) < 9 {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(buf[offset:], v.PathID[:])
	if v.Ascending {
		offset += copy(buf[offset:], []byte{1})
	} else {
		offset += copy(buf[offset:], []byte{0})
	}
	return offset, nil
}

func (v *VirtualSnakeTeardown) UnmarshalBinary(buf []byte) (int, error) {
	if len(buf) < 9 {
		return 0, fmt.Errorf("buffer too small")
	}
	offset := 0
	offset += copy(v.PathID[:], buf[offset:])
	v.Ascending = buf[offset] == 1
	offset += 1
	return offset, nil
}
