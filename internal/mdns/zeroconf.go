package mdns

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/grandcat/zeroconf"
)

const localDomain = "local."

type ZeroconfBrowser struct{}

func NewZeroconfBrowser(_ context.Context) *ZeroconfBrowser {
	return &ZeroconfBrowser{}
}

func (*ZeroconfBrowser) Browse(ctx context.Context, service string) ([]Record, error) {
	// A resolver shuts its sockets down when a browse ends, so each browse needs its own.
	resolver, err := zeroconf.NewResolver(zeroconf.SelectIPTraffic(zeroconf.IPv4))
	if err != nil {
		return nil, fmt.Errorf("failed to create mdns resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	if err := resolver.Browse(ctx, service, localDomain, entries); err != nil {
		return nil, fmt.Errorf("failed to browse %s: %w", service, err)
	}

	// The resolver closes entries once ctx is done.
	var records []Record
	for entry := range entries {
		records = append(records, Record{
			Service:  service,
			HostName: entry.HostName,
			IPv4:     toAddrs(entry),
		})
	}

	return records, nil
}

func toAddrs(entry *zeroconf.ServiceEntry) []netip.Addr {
	var addrs []netip.Addr
	for _, ip := range entry.AddrIPv4 {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		addrs = append(addrs, addr.Unmap())
	}

	return addrs
}
