package domain_test

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/domain"
)

func mustMAC(t *testing.T, text string) domain.MAC {
	t.Helper()

	mac, err := domain.ParseMAC(t.Context(), text)
	require.NoError(t, err)

	return mac
}

func addrs(t *testing.T, texts ...string) []netip.Addr {
	t.Helper()

	result := make([]netip.Addr, 0, len(texts))
	for _, text := range texts {
		result = append(result, netip.MustParseAddr(text))
	}

	return result
}

func TestParseMAC(t *testing.T) {
	t.Run("parses a colon separated EUI-48", func(t *testing.T) {
		mac, err := domain.ParseMAC(t.Context(), "aa:bb:cc:dd:ee:ff")

		require.NoError(t, err)
		require.Equal(t, domain.MAC{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}, mac)
	})

	t.Run("rejects garbage", func(t *testing.T) {
		_, err := domain.ParseMAC(t.Context(), "not-a-mac")

		require.ErrorIs(t, err, domain.ErrInvalidMAC)
	})

	t.Run("rejects EUI-64", func(t *testing.T) {
		_, err := domain.ParseMAC(t.Context(), "00:00:00:00:fe:80:00:00")

		require.ErrorIs(t, err, domain.ErrInvalidMAC)
	})
}

func TestMACFromHardwareAddr(t *testing.T) {
	t.Run("rejects short addresses", func(t *testing.T) {
		_, err := domain.MACFromHardwareAddr(t.Context(), net.HardwareAddr{1, 2, 3})

		require.ErrorIs(t, err, domain.ErrInvalidMAC)
	})
}

func TestFormatMAC(t *testing.T) {
	tests := []struct {
		name string
		mac  domain.MAC
		want string
	}{
		{name: "formats with colons", mac: domain.MAC{0, 0x1a, 0x2b, 0x3c, 0x4d, 0x5e}, want: "00:1a:2b:3c:4d:5e"},
		{name: "zero MAC is empty", mac: domain.MAC{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, domain.FormatMAC(t.Context(), tt.mac))
		})
	}
}

func TestOUIOf(t *testing.T) {
	require.Equal(t, domain.OUI{0xaa, 0xbb, 0xcc}, domain.OUIOf(t.Context(), mustMAC(t, "aa:bb:cc:dd:ee:ff")))
}

func TestIsLocallyAdministered(t *testing.T) {
	require.True(t, domain.IsLocallyAdministered(t.Context(), mustMAC(t, "da:ff:1d:7d:98:ed")))
	require.False(t, domain.IsLocallyAdministered(t.Context(), mustMAC(t, "00:1a:2b:3c:4d:5e")))
}

func TestLookupVendor(t *testing.T) {
	table := domain.VendorTable{ByOUI: map[domain.OUI]string{{0x00, 0x1a, 0x2b}: "Acme"}}

	tests := []struct {
		name string
		mac  domain.MAC
		want string
	}{
		{name: "known vendor", mac: mustMAC(t, "00:1a:2b:00:00:01"), want: "Acme"},
		{name: "unknown vendor", mac: mustMAC(t, "00:99:99:00:00:01"), want: ""},
		{name: "randomized MAC", mac: mustMAC(t, "02:1a:2b:00:00:01"), want: domain.VendorPrivate},
		{name: "no MAC", mac: domain.MAC{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, domain.LookupVendor(t.Context(), table, tt.mac))
		})
	}
}

func TestApplyVendors(t *testing.T) {
	mac := mustMAC(t, "00:1a:2b:00:00:01")
	table := domain.VendorTable{ByOUI: map[domain.OUI]string{domain.OUIOf(t.Context(), mac): "Acme"}}
	inv := domain.Inventory{Devices: map[domain.MAC]domain.Device{mac: {MAC: mac}}}

	got := domain.ApplyVendors(t.Context(), inv, table)

	require.Equal(t, "Acme", got.Devices[mac].Vendor)
	require.Empty(t, inv.Devices[mac].Vendor, "input must not be mutated")
}

func TestSweepTargets(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		network string
		minBits int
		want    []netip.Addr
	}{
		{
			name:    "skips network, broadcast and our own address",
			addr:    "10.0.0.2",
			network: "10.0.0.0/29",
			minBits: 22,
			want:    addrs(t, "10.0.0.1", "10.0.0.3", "10.0.0.4", "10.0.0.5", "10.0.0.6"),
		},
		{
			name:    "point-to-point keeps both addresses",
			addr:    "10.0.0.0",
			network: "10.0.0.0/31",
			minBits: 22,
			want:    addrs(t, "10.0.0.1"),
		},
		{
			name:    "large networks are narrowed around our address",
			addr:    "172.16.5.9",
			network: "172.16.0.0/16",
			minBits: 30,
			want:    addrs(t, "172.16.5.10"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iface := domain.Interface{Addr: netip.MustParseAddr(tt.addr), Network: netip.MustParsePrefix(tt.network)}

			require.Equal(t, tt.want, domain.SweepTargets(t.Context(), iface, tt.minBits))
		})
	}

	t.Run("a /22 has 1021 targets", func(t *testing.T) {
		iface := domain.Interface{
			Addr:    netip.MustParseAddr("192.168.4.1"),
			Network: netip.MustParsePrefix("192.168.4.0/22"),
		}

		targets := domain.SweepTargets(t.Context(), iface, 22)

		require.Len(t, targets, 1021)
		require.Equal(t, netip.MustParseAddr("192.168.7.254"), targets[len(targets)-1])
	})

	t.Run("IPv6 is not swept", func(t *testing.T) {
		iface := domain.Interface{Addr: netip.MustParseAddr("fe80::1"), Network: netip.MustParsePrefix("fe80::/64")}

		require.Empty(t, domain.SweepTargets(t.Context(), iface, 22))
	})
}

func TestInNetworks(t *testing.T) {
	ifaces := []domain.Interface{{Network: netip.MustParsePrefix("192.168.1.0/24")}}

	require.True(t, domain.InNetworks(t.Context(), netip.MustParseAddr("192.168.1.20"), ifaces))
	require.False(t, domain.InNetworks(t.Context(), netip.MustParseAddr("10.0.0.1"), ifaces))
}

func TestMerge(t *testing.T) {
	early := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	late := early.Add(time.Minute)
	mac := mustMAC(t, "00:1a:2b:00:00:01")
	ip := netip.MustParseAddr("192.168.1.10")

	t.Run("observations of one MAC combine into one device", func(t *testing.T) {
		got := domain.Merge(t.Context(), domain.Inventory{}, []domain.Observation{
			{Source: domain.SourceARP, IP: ip, MAC: mac, SeenAt: late},
			{Source: domain.SourceNeighbor, IP: ip, MAC: mac, SeenAt: early},
		})

		require.Len(t, got.Devices, 1)
		device := got.Devices[mac]
		require.Equal(t, []netip.Addr{ip}, device.IPs)
		require.Equal(t, domain.SourceARP|domain.SourceNeighbor, device.Sources)
		require.Equal(t, early, device.FirstSeen)
		require.Equal(t, late, device.LastSeen)
	})

	t.Run("mDNS observations join the device owning the IP", func(t *testing.T) {
		got := domain.Merge(t.Context(), domain.Inventory{}, []domain.Observation{
			{Source: domain.SourceMDNS, IP: ip, Hostname: "printer.local", Services: []string{"ipp"}, SeenAt: late},
			{Source: domain.SourceMDNS, IP: ip, Hostname: "printer.local", Services: []string{"http"}, SeenAt: late},
			{Source: domain.SourceARP, IP: ip, MAC: mac, SeenAt: early},
		})

		require.Empty(t, got.Unresolved)
		device := got.Devices[mac]
		require.Equal(t, "printer.local", device.Hostname)
		require.Equal(t, []string{"http", "ipp"}, device.Services)
		require.Equal(t, domain.SourceARP|domain.SourceMDNS, device.Sources)
	})

	t.Run("mDNS hosts without a MAC stay unresolved until one appears", func(t *testing.T) {
		first := domain.Merge(t.Context(), domain.Inventory{}, []domain.Observation{
			{Source: domain.SourceMDNS, IP: ip, Hostname: "tv.local", SeenAt: early},
		})
		require.Empty(t, first.Devices)
		require.Equal(t, "tv.local", first.Unresolved[ip].Hostname)

		second := domain.Merge(t.Context(), first, []domain.Observation{
			{Source: domain.SourceNeighbor, IP: ip, MAC: mac, SeenAt: late},
		})

		require.Empty(t, second.Unresolved)
		require.Equal(t, "tv.local", second.Devices[mac].Hostname)
		require.Equal(t, early, second.Devices[mac].FirstSeen)
		require.Len(t, first.Unresolved, 1, "input must not be mutated")
	})

	t.Run("a reassigned IP joins its most recent owner", func(t *testing.T) {
		newMAC := mustMAC(t, "00:1a:2b:00:00:02")
		prev := domain.Merge(t.Context(), domain.Inventory{}, []domain.Observation{
			{Source: domain.SourceNeighbor, IP: ip, MAC: mac, SeenAt: early},
			{Source: domain.SourceNeighbor, IP: ip, MAC: newMAC, SeenAt: late},
		})

		got := domain.Merge(t.Context(), prev, []domain.Observation{
			{Source: domain.SourceMDNS, IP: ip, Hostname: "laptop.local", SeenAt: late},
		})

		require.Equal(t, "laptop.local", got.Devices[newMAC].Hostname)
		require.Empty(t, got.Devices[mac].Hostname)
	})

	t.Run("a claimed unresolved host keeps what was known about it", func(t *testing.T) {
		prev := domain.Inventory{Unresolved: map[netip.Addr]domain.Device{
			ip: {IPs: []netip.Addr{ip}, Vendor: "Acme", Hostname: "tv.local", FirstSeen: early, LastSeen: early},
		}}

		got := domain.Merge(t.Context(), prev, []domain.Observation{
			{Source: domain.SourceNeighbor, IP: ip, MAC: mac, SeenAt: late},
		})

		require.Equal(t, "Acme", got.Devices[mac].Vendor)
		require.Equal(t, "tv.local", got.Devices[mac].Hostname)
	})

	t.Run("an empty hostname does not erase a known one", func(t *testing.T) {
		prev := domain.Merge(t.Context(), domain.Inventory{}, []domain.Observation{
			{Source: domain.SourceMDNS, IP: ip, MAC: mac, Hostname: "nas.local", SeenAt: early},
		})

		got := domain.Merge(
			t.Context(),
			prev,
			[]domain.Observation{{Source: domain.SourceARP, IP: ip, MAC: mac, SeenAt: late}},
		)

		require.Equal(t, "nas.local", got.Devices[mac].Hostname)
	})
}

func TestSortedDevices(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	inv := domain.Inventory{
		Devices: map[domain.MAC]domain.Device{
			mustMAC(t, "00:00:00:00:00:01"): {
				IPs: addrs(t, "10.0.0.3"), Vendor: "zebra", Hostname: "b", LastSeen: now,
			},
			mustMAC(t, "00:00:00:00:00:02"): {
				IPs: addrs(t, "10.0.0.1"), Vendor: "Apple", Hostname: "", LastSeen: now.Add(time.Second),
			},
		},
		Unresolved: map[netip.Addr]domain.Device{
			netip.MustParseAddr("10.0.0.2"): {IPs: addrs(t, "10.0.0.2"), Hostname: "a", LastSeen: now},
		},
	}

	tests := []struct {
		key  domain.SortKey
		want []string
	}{
		{key: domain.SortByIP, want: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}},
		{key: domain.SortByVendor, want: []string{"10.0.0.1", "10.0.0.3", "10.0.0.2"}},
		{key: domain.SortByHostname, want: []string{"10.0.0.2", "10.0.0.3", "10.0.0.1"}},
		{key: domain.SortByLastSeen, want: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}},
	}

	for _, tt := range tests {
		t.Run(domain.SortKeyName(t.Context(), tt.key), func(t *testing.T) {
			var got []string
			for _, device := range domain.SortedDevices(t.Context(), inv, tt.key) {
				got = append(got, domain.PrimaryIP(t.Context(), device).String())
			}

			require.Equal(t, tt.want, got)
		})
	}
}

func TestNextSortKey(t *testing.T) {
	key := domain.SortByIP
	seen := map[string]bool{}
	for range 4 {
		seen[domain.SortKeyName(t.Context(), key)] = true
		key = domain.NextSortKey(t.Context(), key)
	}

	require.Equal(t, domain.SortByIP, key, "cycles back to the first key")
	require.Len(t, seen, 4)
	require.NotContains(t, seen, "unknown")
	require.Equal(t, "unknown", domain.SortKeyName(t.Context(), domain.SortKey(200)))
}

func TestPrimaryIP(t *testing.T) {
	require.False(t, domain.PrimaryIP(t.Context(), domain.Device{}).IsValid())
}

func TestSourceNames(t *testing.T) {
	require.Equal(t, []string{"arp", "mdns"}, domain.SourceNames(t.Context(), domain.SourceMDNS|domain.SourceARP))
	require.Equal(t, []string{"arp", "cache", "mdns"},
		domain.SourceNames(t.Context(), domain.SourceARP|domain.SourceNeighbor|domain.SourceMDNS))
	require.Empty(t, domain.SourceNames(t.Context(), 0))
}
