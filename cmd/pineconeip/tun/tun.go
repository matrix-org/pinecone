package tun

import (
	"fmt"
	"net"

	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/types"

	wgtun "golang.zx2c4.com/wireguard/tun"
)

const TUN_OFFSET_BYTES = 4

type TUN struct {
	r     *router.Router
	iface wgtun.Device
	//	partialToFull map[types.PublicKey]types.PublicKey
	//	mutex         sync.RWMutex
}

/*
func (t *TUN) Lookup(partial types.PublicKey) types.PublicKey {
	t.mutex.RLock()
	pk, ok := t.partialToFull[partial]
	t.mutex.RUnlock()
	if ok {
		return pk
	}
	full, addr, err := t.r.DHTSearch(context.Background(), partial[:], false)
	if err != nil {
		fmt.Println("DHT search failed:", err)
		return types.PublicKey{}
	}
	fmt.Println("DHT search from", addr, "returned", full)
	t.mutex.Lock()
	t.partialToFull[partial] = full
	t.mutex.Unlock()
	return full
}
*/

func AddressForPublicKey(pk types.PublicKey) net.IP {
	a := [16]byte{0xFD}
	copy(a[1:], pk[:])
	return a[:]
}

func PublicKeyForAddress(a net.IP) types.PublicKey {
	if a[0] != 0xFD {
		return types.PublicKey{}
	}
	pk := types.PublicKey{}
	copy(pk[:], a[1:])
	return pk
}

func NewTUN(r *router.Router) (*TUN, error) {
	t := &TUN{
		r: r,
		//	partialToFull: make(map[types.PublicKey]types.PublicKey),
	}
	addr := AddressForPublicKey(r.PublicKey())
	if err := t.setup(addr); err != nil {
		return nil, fmt.Errorf("t.setup: %w", err)
	}
	go t.read()
	go t.write()
	return t, nil
}

func (t *TUN) read() {
	var buf [TUN_OFFSET_BYTES + 65536]byte
	for {
		n, err := t.iface.Read(buf[:], TUN_OFFSET_BYTES)
		if n <= TUN_OFFSET_BYTES || err != nil {
			fmt.Println("Error reading TUN:", err)
			_ = t.iface.Flush()
			continue
		}
		bs := buf[TUN_OFFSET_BYTES : TUN_OFFSET_BYTES+n]
		if bs[0]&0xf0 != 0x60 {
			continue
		}
		dst := net.IP(bs[24:40])
		pk := PublicKeyForAddress(dst)
		if dst[0] != 0xFD {
			continue
		}
		ns, err := t.r.WriteTo(bs, pk)
		if err != nil {
			fmt.Println("t.r.WriteTo:", err)
			continue
		}
		if ns < n {
			fmt.Println("Wrote", ns, "bytes but should have wrote", n, "bytes")
			continue
		}
	}
}

func (t *TUN) write() {
	var buf [TUN_OFFSET_BYTES + 65536]byte
	for {
		n, _, err := t.r.ReadFrom(buf[TUN_OFFSET_BYTES:])
		if err != nil {
			fmt.Println("Error reading Pinecone:", err)
			continue
		}
		_, err = t.iface.Write(buf[:TUN_OFFSET_BYTES+n], TUN_OFFSET_BYTES)
		if err != nil {
			fmt.Println("Error writing TUN:", err)
		}
	}
}
