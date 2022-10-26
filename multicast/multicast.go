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

package multicast

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const MulticastIPv4GroupAddr = "224.0.0.114"
const MulticastIPv6GroupAddr = "[ff02::114]"
const MulticastGroupPort = 60606

type InterfaceInfo struct {
	Name         string
	Index        int
	Mtu          int
	Up           bool
	Broadcast    bool
	Loopback     bool
	PointToPoint bool
	Multicast    bool
	Addrs        string
}

type AltInterface struct {
	iface net.Interface
	addrs []net.Addr
}

type Multicast struct {
	r                 *router.Router
	log               types.Logger
	ctx               context.Context
	cancel            context.CancelFunc
	id                string
	started           atomic.Bool
	interfaces        sync.Map // -> *multicastInterface
	dialling          sync.Map
	listener          net.Listener
	dialer            net.Dialer
	tcpLC             net.ListenConfig
	udpLC             net.ListenConfig
	altInterfaces     map[string]AltInterface
	interfaceCallback func()
	callbackMutex     sync.Mutex
}

type multicastInterface struct {
	context context.Context
	cancel  context.CancelFunc
	net.Interface
}

func NewMulticast(
	log types.Logger, r *router.Router,
) *Multicast {
	public := r.PublicKey()
	m := &Multicast{
		r:   r,
		log: log,
		id:  hex.EncodeToString(public[:]),
	}
	m.tcpLC = net.ListenConfig{
		Control: m.tcpOptions,
	}
	m.udpLC = net.ListenConfig{
		Control: m.udpOptions,
	}
	m.dialer = net.Dialer{
		Control: m.tcpOptions,
		Timeout: time.Second * 5,
	}
	return m
}

func (m *Multicast) RegisterNetworkCallback(intfCallback func() []InterfaceInfo) {
	if intfCallback == nil {
		return
	}

	m.callbackMutex.Lock()
	defer m.callbackMutex.Unlock()
	// Assign the callback function used to obtain current interface information.
	m.interfaceCallback = func() {
		// Save a reference to the previously registered interfaces.
		oldInterfaces := m.altInterfaces
		// Clear out any previously registered interfaces.
		m.altInterfaces = make(map[string]AltInterface)

		// Register each returned interface.
		for _, intf := range intfCallback() {
			m.registerInterface(intf)
		}

		// If any of the previously registered interfaces that were being used for
		// multicast discovery are no longer present, cancel their context/s so they
		// are cleaned up appropriately.
		for _, intf := range oldInterfaces {
			if _, ok := m.altInterfaces[intf.iface.Name]; !ok {
				if v, ok := m.interfaces.Load(intf.iface.Name); ok {
					mi := v.(*multicastInterface)
					mi.cancel()
				}
			}
		}
	}
}

func (m *Multicast) registerInterface(info InterfaceInfo) {
	iface := AltInterface{
		net.Interface{
			Name:  info.Name,
			Index: info.Index,
			MTU:   info.Mtu,
		},
		[]net.Addr{},
	}

	if info.Up {
		iface.iface.Flags |= net.FlagUp
	}
	if info.Broadcast {
		iface.iface.Flags |= net.FlagBroadcast
	}
	if info.Loopback {
		iface.iface.Flags |= net.FlagLoopback
	}
	if info.PointToPoint {
		iface.iface.Flags |= net.FlagPointToPoint
	}
	if info.Multicast {
		iface.iface.Flags |= net.FlagMulticast
	}

	info.Addrs = strings.Trim(info.Addrs, " \n")
	for _, addr := range strings.Split(info.Addrs, " ") {
		addr = strings.Split(addr, "/")[0]
		ip, err := net.ResolveIPAddr("ip", addr)
		if err == nil {
			iface.addrs = append(iface.addrs, ip)
		}
	}

	m.altInterfaces[iface.iface.Name] = iface
	m.log.Println("Registered interface ", iface.iface.Name)
}

func (m *Multicast) Start() {
	if !m.started.CAS(false, true) {
		return
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())

	var err error
	m.listener, err = m.tcpLC.Listen(m.ctx, "tcp", "[::]:0")
	if err != nil {
		panic(err)
	}
	m.log.Println("Listening on", m.listener.Addr())
	go m.accept(m.listener)

	go func() {
		for {
			select {
			case <-m.ctx.Done():
				return
			default:
			}

			intfs := []net.Interface{}
			func() {
				m.callbackMutex.Lock()
				defer m.callbackMutex.Unlock()
				if m.interfaceCallback != nil {
					m.interfaceCallback()
					for _, iface := range m.altInterfaces {
						intfs = append(intfs, iface.iface)
					}
				} else {
					intfs, err = net.Interfaces()
					if err != nil {
						return
					}
				}
			}()

			for _, intf := range intfs {
				unsuitable := intf.Flags&net.FlagUp == 0 ||
					intf.Flags&net.FlagMulticast == 0 ||
					intf.Flags&net.FlagPointToPoint != 0

				if v, ok := m.interfaces.Load(intf.Name); ok {
					if unsuitable {
						mi := v.(*multicastInterface)
						mi.cancel()
						m.interfaces.Delete(intf.Name)
					}
				} else {
					if !unsuitable {
						ctx, cancel := context.WithCancel(context.Background())
						mi := &multicastInterface{ctx, cancel, intf}
						go m.startIPv6(mi)
						go m.startIPv4(mi)
					}
				}
			}

			time.Sleep(time.Second * 10)
		}
	}()
}

func (m *Multicast) Stop() {
	if !m.started.CAS(true, false) {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	if m.listener != nil {
		m.listener.Close()
	}
}

func (m *Multicast) accept(listener net.Listener) {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				m.log.Println("m.listener.Accept:", err)
			}
			return
		}

		go func(conn net.Conn) {
			tcpconn, ok := conn.(*net.TCPConn)
			if !ok {
				m.log.Println("Not TCPConn")
				return
			}

			tcpaddr, ok := conn.RemoteAddr().(*net.TCPAddr)
			if !ok {
				m.log.Println("Not TCPAddr")
				return
			}

			if !m.started.Load() {
				_ = conn.Close()
				return
			}

			if err := m.tcpGeneralOptions(tcpconn); err != nil {
				m.log.Println("m.tcpSocketOptions: %w", err)
			}

			if _, err := m.r.Connect(
				tcpconn,
				router.ConnectionZone(tcpaddr.Zone),
				router.ConnectionPeerType(router.PeerTypeMulticast),
			); err != nil {
				//m.log.Println("m.s.AuthenticatedConnect:", err)
				_ = conn.Close()
			}
		}(conn)
	}
}

func (m *Multicast) startIPv4(intf *multicastInterface) {
	groupAddrPort := fmt.Sprintf("%s:%d", MulticastIPv4GroupAddr, MulticastGroupPort)
	addr, err := net.ResolveUDPAddr("udp4", groupAddrPort)
	if err != nil {
		// m.log.Printf("net.ResolveUDPAddr (%s): %s, ignoring interface\n", intf.Name, err)
		return
	}
	listenString := fmt.Sprintf("0.0.0.0:%d", MulticastGroupPort)
	conn, err := m.udpLC.ListenPacket(m.ctx, "udp4", listenString)
	if err != nil {
		// m.log.Printf("lc.ListenPacket (%s): %s, ignoring interface\n", intf.Name, err)
		return
	}
	sock := ipv4.NewPacketConn(conn)
	if err := sock.JoinGroup(&intf.Interface, addr); err != nil {
		// m.log.Printf("sock.JoinGroup (%s): %s, ignoring interface\n", intf.Name, err)
		return
	}
	addr.Zone = intf.Name
	ifaddrs := []net.Addr{}
	for _, v := range m.altInterfaces {
		if v.iface.Name == intf.Name {
			ifaddrs = v.addrs
			break
		}
	}
	if len(ifaddrs) == 0 {
		ifaddrs, err = intf.Addrs()
		if err != nil {
			// m.log.Printf("intf.Addrs (%s): %s, ignoring interface\n", intf.Name, err)
			return
		}
	}
	var srcaddr net.IP
	for _, ifaddr := range ifaddrs {
		srcaddr, _, err = net.ParseCIDR(ifaddr.String())
		if err != nil {
			srcaddr = net.ParseIP(strings.Split(ifaddr.String(), "%")[0])
			if srcaddr == nil {
				// m.log.Println("Failed parsing ifaddr", err)
				continue
			}
		}
		if !srcaddr.IsGlobalUnicast() || srcaddr.To4() == nil {
			srcaddr = nil
			continue
		}
		break
	}
	if srcaddr == nil {
		// m.log.Println("No valid srcaddr found for iface", intf.Name)
		return
	}
	m.log.Printf("Multicast discovery enabled on %s (%s)\n", intf.Name, srcaddr.String())
	m.interfaces.Store(intf.Name, intf)
	go m.advertise(intf, conn, addr)
	go m.listen(intf, conn, &net.TCPAddr{
		IP:   srcaddr,
		Zone: addr.Zone,
	})
}

func (m *Multicast) startIPv6(intf *multicastInterface) {
	groupAddrPort := fmt.Sprintf("%s:%d", MulticastIPv6GroupAddr, MulticastGroupPort)
	addr, err := net.ResolveUDPAddr("udp6", groupAddrPort)
	if err != nil {
		//m.log.Printf("net.ResolveUDPAddr (%s): %s, ignoring interface\n", intf.Name, err)
		return
	}
	listenString := fmt.Sprintf("[::]:%d", MulticastGroupPort)
	conn, err := m.udpLC.ListenPacket(m.ctx, "udp6", listenString)
	if err != nil {
		//m.log.Printf("lc.ListenPacket (%s): %s, ignoring interface\n", intf.Name, err)
		return
	}
	sock := ipv6.NewPacketConn(conn)
	if err := sock.JoinGroup(&intf.Interface, addr); err != nil {
		//m.log.Printf("sock.JoinGroup (%s): %s, ignoring interface\n", intf.Name, err)
		return
	}
	addr.Zone = intf.Name
	ifaddrs := []net.Addr{}
	for _, v := range m.altInterfaces {
		if v.iface.Name == intf.Name {
			ifaddrs = v.addrs
			break
		}
	}
	if len(ifaddrs) == 0 {
		ifaddrs, err = intf.Addrs()
		if err != nil {
			//m.log.Printf("intf.Addrs (%s): %s, ignoring interface\n", intf.Name, err)
			return
		}
	}
	var srcaddr net.IP
	for _, ifaddr := range ifaddrs {
		srcaddr, _, err = net.ParseCIDR(ifaddr.String())
		if err != nil {
			srcaddr = net.ParseIP(strings.Split(ifaddr.String(), "%")[0])
			if srcaddr == nil {
				// m.log.Println("Failed parsing ifaddr", err)
				continue
			}
		}
		if !srcaddr.IsLinkLocalUnicast() || srcaddr.To4() != nil {
			srcaddr = nil
			continue
		}
		break

	}
	if srcaddr == nil {
		return
	}
	m.log.Printf("Multicast discovery enabled on %s (%s)\n", intf.Name, srcaddr.String())
	m.interfaces.Store(intf.Name, intf)
	go m.advertise(intf, conn, addr)
	go m.listen(intf, conn, &net.TCPAddr{
		IP:   srcaddr,
		Zone: addr.Zone,
	})
}

func (m *Multicast) advertise(intf *multicastInterface, conn net.PacketConn, addr net.Addr) {
	defer m.interfaces.Delete(intf.Name)
	// defer m.log.Println("Stop advertising on", intf.Name)
	tcpaddr, _ := m.listener.Addr().(*net.TCPAddr)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(tcpaddr.Port))
	ticker := time.NewTicker(time.Second * 2)
	first := make(chan struct{}, 1)
	first <- struct{}{}
	ourPublicKey := m.r.PublicKey()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-intf.context.Done():
			return
		case <-ticker.C:
		case <-first:
		}
		_, err := conn.WriteTo(
			append(ourPublicKey[:], portBytes...),
			addr,
		)
		if err != nil {
			//m.log.Println("conn.WriteTo:", err)
			continue
		}
	}
}

func (m *Multicast) listen(intf *multicastInterface, conn net.PacketConn, srcaddr net.Addr) {
	defer m.interfaces.Delete(intf.Name)
	// defer m.log.Println("Stop listening on", intf.Name)
	dialer := m.dialer
	dialer.LocalAddr = srcaddr
	dialer.Control = m.tcpOptions
	buf := make([]byte, 512)
	ourPublicKey := m.r.PublicKey()
	neighborKey := types.PublicKey{}
	publicKey := buf[:ed25519.PublicKeySize]
	listenPort := buf[ed25519.PublicKeySize : ed25519.PublicKeySize+2]
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-intf.context.Done():
			return
		default:
		}

		n, addr, err := conn.ReadFrom(buf)
		if err != nil || n < ed25519.PublicKeySize+2 {
			// m.log.Println("conn.ReadFrom:", err)
			intf.cancel()
			continue
		}

		copy(neighborKey[:], publicKey)
		if neighborKey == ourPublicKey {
			continue
		}

		udpaddr, ok := addr.(*net.UDPAddr)
		if !ok {
			continue
		}

		if m.r.IsConnected(neighborKey, udpaddr.Zone) {
			continue
		}

		tcpaddr := &net.TCPAddr{
			IP:   udpaddr.IP,
			Port: int(binary.BigEndian.Uint16(listenPort)),
			Zone: udpaddr.Zone,
		}

		if !m.started.Load() {
			return
		}

		straddr := tcpaddr.String()
		if _, ok := m.dialling.LoadOrStore(straddr, true); ok {
			continue
		}

		go func() {
			defer m.dialling.Delete(straddr)

			conn, err := dialer.Dial("tcp", straddr)
			if err != nil {
				// m.log.Println("dialer.Dial:", err)
				return
			}

			if !m.started.Load() {
				conn.Close()
				return
			}

			tcpconn := conn.(*net.TCPConn)
			if err := m.tcpGeneralOptions(tcpconn); err != nil {
				m.log.Println("m.tcpSocketOptions: %w", err)
			}

			if _, err := m.r.Connect(
				tcpconn,
				router.ConnectionZone(udpaddr.Zone),
				router.ConnectionPeerType(router.PeerTypeMulticast),
			); err != nil {
				m.log.Println("m.s.AuthenticatedConnect:", err)
				_ = conn.Close()
			}
		}()
	}
}

func (m *Multicast) tcpGeneralOptions(tcpconn *net.TCPConn) error {
	if err := tcpconn.SetNoDelay(true); err != nil {
		return fmt.Errorf("tcpconn.SetNoDelay: %w", err)
	}
	if err := tcpconn.SetKeepAlivePeriod(time.Second * 3); err != nil {
		return fmt.Errorf("tcpconn.SetKeepAlivePeriod: %w", err)
	}
	if err := tcpconn.SetKeepAlive(true); err != nil {
		return fmt.Errorf("tcpconn.SetKeepAlive: %w", err)
	}
	return nil
}
