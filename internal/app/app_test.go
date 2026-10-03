package app_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/app"
	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/mocks"
)

func TestBrowse(t *testing.T) {
	newDeps := func(t *testing.T) app.BrowseDeps {
		t.Helper()

		scanner := mocks.NewMockScanner(t)
		scanner.EXPECT().Scan(mock.Anything, mock.Anything).Return(domain.ScanResult{}, nil).Maybe()

		return app.BrowseDeps{
			Logger:  slog.New(slog.DiscardHandler),
			Scanner: scanner,
			UI:      &config.UIConfig{RefreshInterval: time.Hour},
		}
	}

	t.Run("returns when the user quits", func(t *testing.T) {
		err := app.Browse(t.Context(), newDeps(t), strings.NewReader("q"), io.Discard)

		require.NoError(t, err)
	})

	t.Run("stops when the context is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := app.Browse(ctx, newDeps(t), strings.NewReader(""), io.Discard)

		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestUpdateVendors(t *testing.T) {
	newDeps := func(t *testing.T) (app.UpdateDeps, *mocks.MockVendorUpdater) {
		t.Helper()

		updater := mocks.NewMockVendorUpdater(t)

		return app.UpdateDeps{
			Logger:  slog.New(slog.DiscardHandler),
			Updater: updater,
			OUI:     &config.OUIConfig{CachePath: "/cache/oui.csv"},
		}, updater
	}

	t.Run("reports the updated registry", func(t *testing.T) {
		deps, updater := newDeps(t)
		updater.EXPECT().Update(mock.Anything).Return(42, nil)
		var out bytes.Buffer

		err := app.UpdateVendors(t.Context(), deps, &out)

		require.NoError(t, err)
		require.Equal(t, "Saved 42 vendors to /cache/oui.csv\n", out.String())
	})

	t.Run("returns update failures", func(t *testing.T) {
		deps, updater := newDeps(t)
		boom := errors.New("boom")
		updater.EXPECT().Update(mock.Anything).Return(0, boom)

		err := app.UpdateVendors(t.Context(), deps, &bytes.Buffer{})

		require.ErrorIs(t, err, boom)
	})

	t.Run("returns output failures", func(t *testing.T) {
		deps, updater := newDeps(t)
		updater.EXPECT().Update(mock.Anything).Return(1, nil)

		err := app.UpdateVendors(t.Context(), deps, failingWriter{})

		require.Error(t, err)
	})
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("closed")
}
