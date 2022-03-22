package types

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
)

const VirtualSnakePathIDLength = 8

type VirtualSnakePathID [VirtualSnakePathIDLength]byte
type VirtualSnakePathSig [ed25519.SignatureSize]byte

func (p VirtualSnakePathID) MarshalJSON() ([]byte, error) {
	return []byte("\"" + hex.EncodeToString(p[:]) + "\""), nil
}

type VirtualSnakeBootstrap struct {
	PathID    VirtualSnakePathID
	SourceSig VirtualSnakePathSig
	Root
	Failing    byte
	Signatures []BootstrapSignature
}

func (v *VirtualSnakeBootstrap) MarshalBinary(buf []byte) (int, error) {
	// TODO : Add hop signatures size to below check
	if len(buf) < VirtualSnakePathIDLength+v.Root.Length()+ed25519.SignatureSize+1 {
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
	buf[offset] = v.Failing
	offset += 1
	for _, sig := range v.Signatures {
		n, err := sig.MarshalBinary(buf[offset:])
		if err != nil {
			return 0, fmt.Errorf("sig.MarshalBinary: %w", err)
		}
		offset += n
	}
	return offset, nil
}

func (v *VirtualSnakeBootstrap) UnmarshalBinary(buf []byte) (int, error) {
	// TODO : Add hop signatures size to below check
	if len(buf) < VirtualSnakePathIDLength+v.Root.MinLength()+ed25519.SignatureSize+1 {
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
	v.Failing = buf[offset]
	offset += 1
	remaining := buf[offset:]
	for i := uint64(0); len(remaining) >= BootstrapSignatureSize; i++ {
		var signature BootstrapSignature
		n, err := signature.UnmarshalBinary(remaining[:])
		if err != nil {
			return 0, fmt.Errorf("signature.UnmarshalBinary: %w", err)
		}
		if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
			if !ed25519.Verify(signature.PublicKey[:], buf[:len(buf)-len(remaining)], signature.Signature[:]) {
				return 0, fmt.Errorf("signature verification failed for hop %d", i)
			}
		}
		v.Signatures = append(v.Signatures, signature)
		remaining = remaining[n:]
	}

	return offset, nil
}

func (v *VirtualSnakeBootstrap) Sign(privKey ed25519.PrivateKey) error {
	var body [65535]byte
	n, err := v.MarshalBinary(body[:])
	if err != nil {
		return fmt.Errorf("v.MarshalBinary: %w", err)
	}
	sig := BootstrapSignature{}
	copy(sig.PublicKey[:], privKey.Public().(ed25519.PublicKey))
	if _, ok := os.LookupEnv("PINECONE_DISABLE_SIGNATURES"); !ok {
		copy(sig.Signature[:], ed25519.Sign(privKey, body[:n]))
	}
	v.Signatures = append(v.Signatures, sig)

	return nil
}

func (v *VirtualSnakeBootstrap) SanityCheck(from PublicKey, thisNode PublicKey, source PublicKey) error {
	sigs := make(map[PublicKey]struct{}, len(v.Signatures))
	for index, sig := range v.Signatures {
		if sig.PublicKey.CompareTo(thisNode) == 0 {
			return fmt.Errorf("update already contains this node's signature")
		}
		if _, ok := sigs[sig.PublicKey]; ok {
			return fmt.Errorf("update contains routing loop")
		}
		// NOTE : ensure each sig is a lower key than the last
		if index > 0 && sig.PublicKey.CompareTo(v.Signatures[index-1].PublicKey) >= 0 {
			return fmt.Errorf("update contains an invalid sequence of candidates")
		}
		// NOTE : ensure each sig is a higher key than the source
		if sig.PublicKey.CompareTo(source) <= 0 {
			return fmt.Errorf("update contains an invalid candidate")
		}
		sigs[sig.PublicKey] = struct{}{}
	}
	return nil
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
