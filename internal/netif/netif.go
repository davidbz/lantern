// Package netif lists the local interfaces lantern can scan.
package netif

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/davidbz/lantern/internal/domain"
)

type Lister struct{}

func NewLister(_ context.Context) *Lister {
	return &Lister{}
}

// List returns every up, non-loopback Ethernet-like interface with an IPv4 address, once per address.
func (*Lister) List(ctx context.Context) ([]domain.Interface, error) {
	systemIfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	var ifaces []domain.Interface
	for _, systemIface := range systemIfaces {
		addrs, err := systemIface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("failed to list addresses of %s: %w", systemIface.Name, err)
		}
		ifaces = append(ifaces, Convert(ctx, systemIface, addrs)...)
	}

	return ifaces, nil
}

// Convert maps one OS interface to the scannable domain interfaces it carries.
func Convert(ctx context.Context, systemIface net.Interface, addrs []net.Addr) []domain.Interface {
	if systemIface.Flags&net.FlagUp == 0 || systemIface.Flags&net.FlagLoopback != 0 {
		return nil
	}

	mac, err := domain.MACFromHardwareAddr(ctx, systemIface.HardwareAddr)
	if err != nil {
		return nil // not Ethernet-like (tunnels, WireGuard): nothing to sweep
	}

	var ifaces []domain.Interface
	for _, addr := range addrs {
		prefix, ok := ipv4Prefix(addr)
		if !ok {
			continue
		}
		ifaces = append(ifaces, domain.Interface{
			Name:    systemIface.Name,
			MAC:     mac,
			Addr:    prefix.Addr(),
			Network: prefix.Masked(),
		})
	}

	return ifaces
}

func ipv4Prefix(addr net.Addr) (netip.Prefix, bool) {
	ipNet, ok := addr.(*net.IPNet)
	if !ok {
		return netip.Prefix{}, false
	}

	ip, ok := netip.AddrFromSlice(ipNet.IP)
	if !ok {
		return netip.Prefix{}, false
	}
	ip = ip.Unmap()
	if !ip.Is4() || ip.IsLinkLocalUnicast() {
		return netip.Prefix{}, false
	}

	ones, _ := ipNet.Mask.Size()

	return netip.PrefixFrom(ip, ones), true
}
