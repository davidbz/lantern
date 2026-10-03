package neighbor_test

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/neighbor"
)

const table = `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.1      0x1         0x2         00:1a:2b:3c:4d:5e     *        eth0
192.168.1.7      0x1         0x0         00:00:00:00:00:00     *        eth0
192.168.1.9      0x1         0x2         00:00:00:00:00:00     *        eth0
10.8.0.4         0x1         0x2         00:1a:2b:3c:4d:5f     *        docker0
192.168.1.20     0x1         0x2         not-a-mac             *        eth0
bogus            0x1         0x2         00:1a:2b:3c:4d:60     *        eth0
truncated line
`

func TestParseTable(t *testing.T) {
	ifaces := []domain.Interface{{Name: "eth0"}}
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	observations, err := neighbor.ParseTable(t.Context(), strings.NewReader(table), ifaces, now)

	require.NoError(t, err)
	require.Equal(t, []domain.Observation{{
		Source: domain.SourceNeighbor,
		IP:     netip.MustParseAddr("192.168.1.1"),
		MAC:    domain.MAC{0x00, 0x1a, 0x2b, 0x3c, 0x4d, 0x5e},
		SeenAt: now,
	}}, observations)
}

func TestReaderDiscover(t *testing.T) {
	ifaces := []domain.Interface{{Name: "eth0"}}

	t.Run("reads the configured table", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "arp")
		require.NoError(t, os.WriteFile(path, []byte(table), 0o600))
		reader := neighbor.NewReader(t.Context(), &config.ScanConfig{NeighborTablePath: path})

		observations, err := reader.Discover(t.Context(), ifaces)

		require.NoError(t, err)
		require.Len(t, observations, 1)
	})

	t.Run("a missing table means the source is unavailable", func(t *testing.T) {
		reader := neighbor.NewReader(
			t.Context(),
			&config.ScanConfig{NeighborTablePath: filepath.Join(t.TempDir(), "missing")},
		)

		_, err := reader.Discover(t.Context(), ifaces)

		require.ErrorIs(t, err, domain.ErrSourceUnavailable)
	})

	t.Run("a table that can't be opened is an error", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(blocker, nil, 0o600))
		reader := neighbor.NewReader(t.Context(), &config.ScanConfig{NeighborTablePath: filepath.Join(blocker, "arp")})

		_, err := reader.Discover(t.Context(), ifaces)

		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrSourceUnavailable)
	})

	t.Run("an unreadable table is an error", func(t *testing.T) {
		reader := neighbor.NewReader(t.Context(), &config.ScanConfig{NeighborTablePath: t.TempDir()})

		_, err := reader.Discover(t.Context(), ifaces)

		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrSourceUnavailable)
	})
}
