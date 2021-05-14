package tun

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"

	wgtun "golang.zx2c4.com/wireguard/tun"
)

// Configures the "utun" adapter with the correct IPv6 address and MTU.
func (tun *TUN) setup(addr net.IP) error {
	iface, err := wgtun.CreateTUN("utun", 65000)
	if err != nil {
		panic(err)
	}
	tun.iface = iface
	return tun.setupAddress(addr.String())
}

const (
	darwin_SIOCAIFADDR_IN6       = 2155899162 // netinet6/in6_var.h
	darwin_IN6_IFF_NODAD         = 0x0020     // netinet6/in6_var.h
	darwin_IN6_IFF_SECURED       = 0x0400     // netinet6/in6_var.h
	darwin_ND6_INFINITE_LIFETIME = 0xFFFFFFFF // netinet6/nd6.h
)

// nolint:structcheck
type in6_addrlifetime struct {
	ia6t_expire    float64 // nolint:unused
	ia6t_preferred float64 // nolint:unused
	ia6t_vltime    uint32
	ia6t_pltime    uint32
}

// nolint:structcheck
type sockaddr_in6 struct {
	sin6_len      uint8
	sin6_family   uint8
	sin6_port     uint8  // nolint:unused
	sin6_flowinfo uint32 // nolint:unused
	sin6_addr     [8]uint16
	sin6_scope_id uint32 // nolint:unused
}

// nolint:structcheck
type in6_aliasreq struct {
	ifra_name       [16]byte
	ifra_addr       sockaddr_in6
	ifra_dstaddr    sockaddr_in6 // nolint:unused
	ifra_prefixmask sockaddr_in6
	ifra_flags      uint32
	ifra_lifetime   in6_addrlifetime
}

// Sets the IPv6 address of the utun adapter. On Darwin/macOS this is done using
// a system socket and making direct syscalls to the kernel.
func (tun *TUN) setupAddress(addr string) error {
	var fd int
	var err error

	if fd, err = unix.Socket(unix.AF_INET6, unix.SOCK_DGRAM, 0); err != nil {
		return fmt.Errorf("unix.Socket: %w", err)
	}

	ifname, err := tun.iface.Name()
	if err != nil {
		return fmt.Errorf("tun.iface.Name: %w", err)
	}

	var ar in6_aliasreq
	copy(ar.ifra_name[:], []byte(ifname))

	ar.ifra_prefixmask.sin6_len = uint8(unsafe.Sizeof(ar.ifra_prefixmask))
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b, uint16(0xFE00))
	ar.ifra_prefixmask.sin6_addr[0] = uint16(binary.BigEndian.Uint16(b))

	ar.ifra_addr.sin6_len = uint8(unsafe.Sizeof(ar.ifra_addr))
	ar.ifra_addr.sin6_family = unix.AF_INET6
	parts := strings.Split(strings.Split(addr, "/")[0], ":")
	for i := 0; i < 8; i++ {
		addr, _ := strconv.ParseUint(parts[i], 16, 16)
		b := make([]byte, 16)
		binary.LittleEndian.PutUint16(b, uint16(addr))
		ar.ifra_addr.sin6_addr[i] = uint16(binary.BigEndian.Uint16(b))
	}

	ar.ifra_flags |= darwin_IN6_IFF_NODAD
	ar.ifra_flags |= darwin_IN6_IFF_SECURED

	ar.ifra_lifetime.ia6t_vltime = darwin_ND6_INFINITE_LIFETIME
	ar.ifra_lifetime.ia6t_pltime = darwin_ND6_INFINITE_LIFETIME

	fmt.Println("Interface name:", ifname)
	fmt.Println("Interface address:", addr)

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(darwin_SIOCAIFADDR_IN6), uintptr(unsafe.Pointer(&ar))); errno != 0 {
		err = errno
		return fmt.Errorf("darwin_SIOCAIFADDR_IN6: %w", err)
	}

	return err
}
