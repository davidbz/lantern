package netif_test

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/netif"
)

func ipNet(t *testing.T, cidr string) *net.IPNet {
	t.Helper()

	ip, network, err := net.ParseCIDR(cidr)
	require.NoError(t, err)
	network.IP = ip

	return network
}

func TestConvert(t *testing.T) {
	hw := net.HardwareAddr{0x00, 0x1a, 0x2b, 0x3c, 0x4d, 0x5e}
	up := net.Interface{Name: "eth0", Flags: net.FlagUp, HardwareAddr: hw}

	t.Run("keeps IPv4 addresses with their masked network", func(t *testing.T) {
		got := netif.Convert(t.Context(), up, []net.Addr{
			ipNet(t, "192.168.1.10/24"),
			ipNet(t, "fe80::1/64"),
			ipNet(t, "169.254.3.4/16"),
			&net.IPAddr{IP: net.ParseIP("10.0.0.1")},
			&net.IPNet{IP: net.IP{10, 0, 0}, Mask: net.CIDRMask(24, 32)},
		})

		require.Equal(t, []domain.Interface{{
			Name:    "eth0",
			MAC:     domain.MAC(hw),
			Addr:    netip.MustParseAddr("192.168.1.10"),
			Network: netip.MustParsePrefix("192.168.1.0/24"),
		}}, got)
	})

	tests := []struct {
		name  string
		iface net.Interface
	}{
		{name: "down", iface: net.Interface{Name: "eth1", HardwareAddr: hw}},
		{name: "loopback", iface: net.Interface{Name: "lo", Flags: net.FlagUp | net.FlagLoopback}},
		{name: "no Ethernet address", iface: net.Interface{Name: "wg0", Flags: net.FlagUp}},
	}
	for _, tt := range tests {
		t.Run("skips "+tt.name+" interfaces", func(t *testing.T) {
			require.Empty(t, netif.Convert(t.Context(), tt.iface, []net.Addr{ipNet(t, "10.0.0.1/24")}))
		})
	}
}

func TestListerList(t *testing.T) {
	ifaces, err := netif.NewLister(t.Context()).List(t.Context())

	require.NoError(t, err)
	for _, iface := range ifaces {
		require.True(t, iface.Addr.Is4())
		require.True(t, iface.Network.Contains(iface.Addr))
	}
}
