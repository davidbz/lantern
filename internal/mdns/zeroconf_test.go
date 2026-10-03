package mdns_test

import (
	"context"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/mdns"
)

func TestZeroconfBrowserBrowse(t *testing.T) {
	const service = "_lantern-test._tcp"

	server, err := zeroconf.Register("lantern-test", service, "local.", 9, nil, nil)
	if err != nil {
		t.Skipf("multicast DNS is unavailable here: %v", err)
	}
	t.Cleanup(server.Shutdown)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	t.Cleanup(cancel)

	records, err := mdns.NewZeroconfBrowser(t.Context()).Browse(ctx, service)

	require.NoError(t, err)
	require.NotEmpty(t, records)
	require.Equal(t, service, records[0].Service)
	require.NotEmpty(t, records[0].HostName)
}
