package domain

import (
	"cmp"
	"context"
	"maps"
	"net/netip"
	"slices"
	"strings"
)

type SortKey uint8

const (
	SortByIP SortKey = iota
	SortByVendor
	SortByHostname
	SortByLastSeen
	sortKeyCount
)

func NextSortKey(_ context.Context, key SortKey) SortKey {
	return (key + 1) % sortKeyCount
}

func SortKeyName(_ context.Context, key SortKey) string {
	switch key {
	case SortByIP:
		return "IP"
	case SortByVendor:
		return "vendor"
	case SortByHostname:
		return "hostname"
	case SortByLastSeen:
		return "last seen"
	case sortKeyCount:
	}

	return "unknown"
}

// SortedDevices flattens the inventory, resolved and unresolved hosts alike, in the requested order.
// Ties are broken by IP so the order is stable between refreshes.
func SortedDevices(ctx context.Context, inv Inventory, key SortKey) []Device {
	devices := slices.Concat(slices.Collect(maps.Values(inv.Devices)), slices.Collect(maps.Values(inv.Unresolved)))
	slices.SortFunc(devices, func(a, b Device) int {
		return cmp.Or(compareBy(a, b, key), PrimaryIP(ctx, a).Compare(PrimaryIP(ctx, b)))
	})

	return devices
}

// PrimaryIP is the lowest IP of a device, or the zero Addr when it has none.
func PrimaryIP(_ context.Context, device Device) netip.Addr {
	if len(device.IPs) == 0 {
		return netip.Addr{}
	}

	return device.IPs[0]
}

// SourceNames lists the sources in a set, in a fixed order.
func SourceNames(_ context.Context, sources SourceKind) []string {
	var names []string
	for _, kind := range []SourceKind{SourceARP, SourceNeighbor, SourceMDNS} {
		if sources&kind != 0 {
			names = append(names, sourceName(kind))
		}
	}

	return names
}

func sourceName(kind SourceKind) string {
	switch kind {
	case SourceARP:
		return "arp"
	case SourceNeighbor:
		return "cache"
	case SourceMDNS:
		return "mdns"
	}

	return "unknown"
}

func compareBy(a, b Device, key SortKey) int {
	switch key {
	case SortByVendor:
		return compareText(a.Vendor, b.Vendor)
	case SortByHostname:
		return compareText(a.Hostname, b.Hostname)
	case SortByLastSeen:
		return b.LastSeen.Compare(a.LastSeen) // most recent first
	case SortByIP, sortKeyCount:
	}

	return 0
}

// compareText orders case-insensitively and puts empty values last.
func compareText(a, b string) int {
	if (a == "") != (b == "") {
		return cmp.Compare(b, a) // whichever is non-empty sorts first
	}

	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}
