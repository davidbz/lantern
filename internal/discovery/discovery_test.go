package discovery_test

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/discovery"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/mocks"
)

type testDeps struct {
	interfaces *mocks.MockInterfaceLister
	first      *mocks.MockSource
	second     *mocks.MockSource
}

func initTest(t *testing.T) (*discovery.Service, *testDeps) {
	t.Helper()

	deps := &testDeps{
		interfaces: mocks.NewMockInterfaceLister(t),
		first:      mocks.NewMockSource(t),
		second:     mocks.NewMockSource(t),
	}
	mac := domain.MAC{0x00, 0x1a, 0x2b, 0x00, 0x00, 0x01}

	service := discovery.NewService(t.Context(), discovery.Params{
		Sources:    []discovery.Source{deps.first, deps.second},
		Interfaces: deps.interfaces,
		Vendors:    domain.VendorTable{ByOUI: map[domain.OUI]string{domain.OUIOf(t.Context(), mac): "Acme"}},
		Config:     &config.ScanConfig{Timeout: time.Second},
	})

	return service, deps
}

func TestServiceScan(t *testing.T) {
	ifaces := []domain.Interface{{Name: "eth0", Network: netip.MustParsePrefix("192.168.1.0/24")}}
	mac := domain.MAC{0x00, 0x1a, 0x2b, 0x00, 0x00, 0x01}
	ip := netip.MustParseAddr("192.168.1.10")

	t.Run("merges every source and resolves vendors", func(t *testing.T) {
		service, deps := initTest(t)
		deps.interfaces.EXPECT().List(mock.Anything).Return(ifaces, nil)
		deps.first.EXPECT().Discover(mock.Anything, ifaces).
			Return([]domain.Observation{{Source: domain.SourceARP, IP: ip, MAC: mac}}, nil)
		deps.second.EXPECT().Discover(mock.Anything, ifaces).
			Return([]domain.Observation{{Source: domain.SourceMDNS, IP: ip, Hostname: "nas.local"}}, nil)

		result, err := service.Scan(t.Context(), domain.Inventory{})

		require.NoError(t, err)
		require.Empty(t, result.Warnings)
		device := result.Inventory.Devices[mac]
		require.Equal(t, "Acme", device.Vendor)
		require.Equal(t, "nas.local", device.Hostname)
		require.Equal(t, domain.SourceARP|domain.SourceMDNS, device.Sources)
	})

	t.Run("sources that cannot run here become warnings", func(t *testing.T) {
		service, deps := initTest(t)
		deps.interfaces.EXPECT().List(mock.Anything).Return(ifaces, nil)
		deps.first.EXPECT().Discover(mock.Anything, ifaces).Return(nil, domain.ErrInsufficientPrivileges)
		deps.second.EXPECT().Discover(mock.Anything, ifaces).
			Return([]domain.Observation{{Source: domain.SourceNeighbor, IP: ip, MAC: mac}}, nil)

		result, err := service.Scan(t.Context(), domain.Inventory{})

		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		require.ErrorIs(t, result.Warnings[0], domain.ErrInsufficientPrivileges)
		require.Contains(t, result.Inventory.Devices, mac)
	})

	t.Run("any other source failure fails the scan", func(t *testing.T) {
		service, deps := initTest(t)
		boom := errors.New("boom")
		deps.interfaces.EXPECT().List(mock.Anything).Return(ifaces, nil)
		deps.first.EXPECT().Discover(mock.Anything, ifaces).Return(nil, boom)
		deps.second.EXPECT().Discover(mock.Anything, ifaces).Return(nil, domain.ErrSourceUnavailable)

		_, err := service.Scan(t.Context(), domain.Inventory{})

		require.ErrorIs(t, err, boom)
	})

	t.Run("fails when interfaces can't be listed", func(t *testing.T) {
		service, deps := initTest(t)
		boom := errors.New("boom")
		deps.interfaces.EXPECT().List(mock.Anything).Return(nil, boom)

		_, err := service.Scan(t.Context(), domain.Inventory{})

		require.ErrorIs(t, err, boom)
	})

	t.Run("fails when there is nothing to scan", func(t *testing.T) {
		service, deps := initTest(t)
		deps.interfaces.EXPECT().List(mock.Anything).Return(nil, nil)

		_, err := service.Scan(t.Context(), domain.Inventory{})

		require.ErrorIs(t, err, discovery.ErrNoInterfaces)
	})
}
