package domain

import (
	"context"
	"encoding/binary"
	"net/netip"
)

const (
	// Point-to-point (/31) and host (/32) prefixes have no network or broadcast address to skip.
	pointToPointBits = 31
)

// SweepTargets lists the addresses to probe on an interface: every host of its network except our own.
// Networks larger than minPrefixBits are narrowed to the minPrefixBits block around our own address.
func SweepTargets(_ context.Context, iface Interface, minPrefixBits int) []netip.Addr {
	if !iface.Addr.Is4() {
		return nil
	}

	bits := max(iface.Network.Bits(), minPrefixBits)
	prefix := netip.PrefixFrom(iface.Addr, bits).Masked()
	first, last := firstHost(prefix), lastHost(prefix)

	var targets []netip.Addr
	for addr := first; addr.IsValid() && addr.Compare(last) <= 0; addr = addr.Next() {
		if addr == iface.Addr {
			continue
		}
		targets = append(targets, addr)
	}

	return targets
}

// InNetworks reports whether ip belongs to any of the interfaces' networks.
func InNetworks(_ context.Context, ip netip.Addr, ifaces []Interface) bool {
	for _, iface := range ifaces {
		if iface.Network.Contains(ip) {
			return true
		}
	}

	return false
}

// firstHost and lastHost skip the network and broadcast addresses, except on point-to-point and host prefixes.
func firstHost(prefix netip.Prefix) netip.Addr {
	if prefix.Bits() >= pointToPointBits {
		return prefix.Addr()
	}

	return prefix.Addr().Next()
}

func lastHost(prefix netip.Prefix) netip.Addr {
	bytes := prefix.Addr().As4()
	binary.BigEndian.PutUint32(bytes[:], binary.BigEndian.Uint32(bytes[:])|^uint32(0)>>prefix.Bits())
	broadcast := netip.AddrFrom4(bytes)
	if prefix.Bits() >= pointToPointBits {
		return broadcast
	}

	return broadcast.Prev()
}
