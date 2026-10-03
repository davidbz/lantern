package mdns_test

import (
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/mdns"
	"github.com/davidbz/lantern/internal/mocks"
)

func TestDiscovererDiscover(t *testing.T) {
	ifaces := []domain.Interface{{Name: "eth0", Network: netip.MustParsePrefix("192.168.1.0/24")}}
	cfg := &config.ScanConfig{MDNSWait: time.Second, MDNSServices: []string{"_ipp._tcp", "_googlecast._tcp"}}
	printer := netip.MustParseAddr("192.168.1.30")

	t.Run("turns records on the scanned networks into observations", func(t *testing.T) {
		browser := mocks.NewMockBrowser(t)
		browser.EXPECT().Browse(mock.Anything, "_ipp._tcp").Return([]mdns.Record{{
			Service:  "_ipp._tcp",
			HostName: "printer.local.",
			IPv4:     []netip.Addr{printer, netip.MustParseAddr("10.9.9.9")},
		}}, nil)
		browser.EXPECT().Browse(mock.Anything, "_googlecast._tcp").Return(nil, nil)

		observations, err := mdns.NewDiscoverer(t.Context(), browser, cfg).Discover(t.Context(), ifaces)

		require.NoError(t, err)
		require.Len(t, observations, 1)
		require.Equal(t, domain.SourceMDNS, observations[0].Source)
		require.Equal(t, printer, observations[0].IP)
		require.True(t, domain.IsZeroMAC(t.Context(), observations[0].MAC))
		require.Equal(t, "printer.local", observations[0].Hostname)
		require.Equal(t, []string{"ipp"}, observations[0].Services)
	})

	t.Run("browse failures fail discovery", func(t *testing.T) {
		browser := mocks.NewMockBrowser(t)
		boom := errors.New("boom")
		browser.EXPECT().Browse(mock.Anything, "_ipp._tcp").Return(nil, boom)
		browser.EXPECT().Browse(mock.Anything, "_googlecast._tcp").Return(nil, nil)

		_, err := mdns.NewDiscoverer(t.Context(), browser, cfg).Discover(t.Context(), ifaces)

		require.ErrorIs(t, err, boom)
	})
}

func TestServiceLabel(t *testing.T) {
	tests := map[string]string{
		"_ipp._tcp":         "ipp",
		"_googlecast._tcp":  "googlecast",
		"_sleep-proxy._udp": "sleep-proxy",
		"plain":             "plain",
	}

	for service, want := range tests {
		t.Run(service, func(t *testing.T) {
			require.Equal(t, want, mdns.ServiceLabel(t.Context(), service))
		})
	}
}

func TestToAddrs(t *testing.T) {
	entry := zeroconf.NewServiceEntry("printer", "_ipp._tcp", "local.")
	entry.AddrIPv4 = []net.IP{net.ParseIP("192.168.1.30"), {1, 2}}

	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.168.1.30")}, mdns.ToAddrs(entry))
}
