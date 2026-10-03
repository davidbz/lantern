package domain

import (
	"context"
	"maps"
	"net/netip"
	"slices"
	"strings"
	"time"
)

// Merge folds observations into a copy of the inventory. Observations carrying a MAC are applied first so that
// MAC-less ones (mDNS) can be joined to a device by IP; the rest stay unresolved until a MAC source claims the IP.
func Merge(ctx context.Context, inv Inventory, observations []Observation) Inventory {
	devices := maps.Clone(inv.Devices)
	if devices == nil {
		devices = make(map[MAC]Device)
	}
	unresolved := maps.Clone(inv.Unresolved)
	if unresolved == nil {
		unresolved = make(map[netip.Addr]Device)
	}

	for _, obs := range observations {
		if IsZeroMAC(ctx, obs.MAC) {
			continue
		}
		devices[obs.MAC] = absorb(ctx, devices[obs.MAC], obs)
	}

	owners := ipOwners(devices)
	for _, obs := range observations {
		if !IsZeroMAC(ctx, obs.MAC) {
			continue
		}
		owner, ok := owners[obs.IP]
		if !ok {
			unresolved[obs.IP] = absorb(ctx, unresolved[obs.IP], obs)
			continue
		}
		devices[owner] = absorb(ctx, devices[owner], obs)
	}

	for ip := range unresolved {
		owner, ok := owners[ip]
		if !ok {
			continue
		}
		devices[owner] = combine(ctx, devices[owner], unresolved[ip])
		delete(unresolved, ip)
	}

	return Inventory{Devices: devices, Unresolved: unresolved}
}

func absorb(ctx context.Context, device Device, obs Observation) Device {
	return combine(ctx, device, Device{
		MAC:       obs.MAC,
		IPs:       []netip.Addr{obs.IP},
		Hostname:  obs.Hostname,
		Vendor:    "",
		Services:  obs.Services,
		Sources:   obs.Source,
		FirstSeen: obs.SeenAt,
		LastSeen:  obs.SeenAt,
	})
}

// combine merges src into dst without mutating either's slices.
func combine(ctx context.Context, dst, src Device) Device {
	merged := dst
	if IsZeroMAC(ctx, merged.MAC) {
		merged.MAC = src.MAC
	}
	if src.Hostname != "" {
		merged.Hostname = src.Hostname
	}
	if src.Vendor != "" {
		merged.Vendor = src.Vendor
	}
	merged.IPs = union(dst.IPs, src.IPs, netip.Addr.Compare)
	merged.Services = union(dst.Services, src.Services, strings.Compare)
	merged.Sources |= src.Sources
	merged.FirstSeen = earliest(dst.FirstSeen, src.FirstSeen)
	if src.LastSeen.After(merged.LastSeen) {
		merged.LastSeen = src.LastSeen
	}

	return merged
}

// ipOwners maps each IP to the device that holds it; when an IP moved between devices the most recent one wins.
func ipOwners(devices map[MAC]Device) map[netip.Addr]MAC {
	owners := make(map[netip.Addr]MAC, len(devices))
	for mac := range devices {
		for _, ip := range devices[mac].IPs {
			current, taken := owners[ip]
			if taken && devices[current].LastSeen.After(devices[mac].LastSeen) {
				continue
			}
			owners[ip] = mac
		}
	}

	return owners
}

func union[T comparable](a, b []T, cmp func(T, T) int) []T {
	merged := slices.Concat(a, b)
	slices.SortFunc(merged, cmp)

	return slices.Compact(merged)
}

func earliest(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}

	return a
}
