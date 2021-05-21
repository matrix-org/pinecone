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
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/matrix-org/pinecone/router"
	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
	"golang.org/x/net/ipv6"
)

const MulticastGroupAddr = "[ff02::114]"
const MulticastGroupPort = 60606

type Multicast struct {
	r          *router.Router
	log        *log.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	id         string
	started    atomic.Bool
	interfaces sync.Map // -> *multicastInterface
	listener   net.Listener
	dialer     net.Dialer
	tcpLC      net.ListenConfig
	udpLC      net.ListenConfig
}

type multicastInterface struct {
	context context.Context
	cancel  context.CancelFunc
	net.Interface
}

func NewMulticast(
	log *log.Logger, r *router.Router,
) *Multicast {
	public := r.PublicKey()
	m := &Multicast{
		r:   r,
		log: log,
		id:  hex.EncodeToString(public[:]),
	}
	m.tcpLC = net.ListenConfig{
		Control:   m.tcpOptions,
		KeepAlive: time.Second,
	}
	m.udpLC = net.ListenConfig{
		Control: m.udpOptions,
	}
	m.dialer = net.Dialer{
		Control:   m.tcpOptions,
		Timeout:   time.Second * 5,
		KeepAlive: time.Second,
	}
	return m
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

			intfs, err := net.Interfaces()
			if err != nil {
				panic(err)
			}

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
						go m.start(mi)
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
			m.log.Println("m.listener.Accept:", err)
			return
		}

		tcpaddr, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok {
			m.log.Println("Not TCPAddr")
			return
		}

		if !m.started.Load() {
			return
		}

		if _, err := m.r.AuthenticatedConnect(conn, tcpaddr.Zone, router.PeerTypeMulticast); err != nil {
			//m.log.Println("m.s.AuthenticatedConnect:", err)
			continue
		}
	}
}

func (m *Multicast) start(intf *multicastInterface) {
	groupAddrPort := fmt.Sprintf("%s:%d", MulticastGroupAddr, MulticastGroupPort)
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
	ifaddrs, err := intf.Addrs()
	if err != nil {
		//m.log.Printf("intf.Addrs (%s): %s, ignoring interface\n", intf.Name, err)
		return
	}
	var srcaddr net.IP
	for _, ifaddr := range ifaddrs {
		srcaddr, _, err = net.ParseCIDR(ifaddr.String())
		if err != nil {
			continue
		}
		if !srcaddr.IsLinkLocalUnicast() || srcaddr.To4() != nil {
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
	//defer m.log.Println("Stop advertising on", intf.Name)
	tcpaddr, _ := m.listener.Addr().(*net.TCPAddr)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(tcpaddr.Port))
	ticker := time.NewTicker(time.Second * 1)
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
	//defer m.log.Println("Stop listening on", intf.Name)
	dialer := m.dialer
	//dialer.LocalAddr = srcaddr
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
			//m.log.Println("conn.ReadFrom:", err)
			intf.cancel()
			continue
		}

		copy(neighborKey[:], publicKey)
		if neighborKey.EqualTo(ourPublicKey) {
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

		peer, err := dialer.Dial("tcp6", tcpaddr.String())
		if err != nil {
			//m.log.Println("dialer.Dial:", err)
			continue
		}

		if !m.started.Load() {
			peer.Close()
			return
		}

		if _, err := m.r.AuthenticatedConnect(peer, udpaddr.Zone, router.PeerTypeMulticast); err != nil {
			m.log.Println("m.s.AuthenticatedConnect:", err)
			continue
		}
	}
}
