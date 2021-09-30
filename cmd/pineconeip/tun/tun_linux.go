package tun

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	wgtun "golang.zx2c4.com/wireguard/tun"
)

func (tun *TUN) setup(addr net.IP) error {
	iface, err := wgtun.CreateTUN("\000", 65000)
	if err != nil {
		panic(err)
	}
	tun.iface = iface
	return tun.setupAddress(addr.String())
}

func (tun *TUN) setupAddress(addr string) error {
	nladdr, err := netlink.ParseAddr(addr + "/8")
	if err != nil {
		return err
	}
	ifname, err := tun.iface.Name()
	if err != nil {
		return err
	}
	nlintf, err := netlink.LinkByName(ifname)
	if err != nil {
		return err
	}
	if err := netlink.AddrAdd(nlintf, nladdr); err != nil {
		return err
	}
	if err := netlink.LinkSetUp(nlintf); err != nil {
		return err
	}
	fmt.Println("Interface name:", ifname)
	fmt.Println("Interface address:", addr)
	return nil
}
