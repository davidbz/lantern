package oui_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/oui"
)

const registry = `Registry,Assignment,Organization Name,Organization Address
MA-L,001A2B,"Acme, Inc.",Somewhere
MA-L,ZZZZZZ,Bad Hex,Nowhere
MA-L,00112,Too Short,Nowhere
MA-L,AABBCC,  Padded Corp  ,Elsewhere
`

func TestParse(t *testing.T) {
	t.Run("maps valid assignments to trimmed names", func(t *testing.T) {
		table, err := oui.Parse(t.Context(), strings.NewReader(registry))

		require.NoError(t, err)
		require.Equal(t, map[domain.OUI]string{
			{0x00, 0x1a, 0x2b}: "Acme, Inc.",
			{0xaa, 0xbb, 0xcc}: "Padded Corp",
		}, table.ByOUI)
	})

	t.Run("a registry without entries is an error", func(t *testing.T) {
		_, err := oui.Parse(t.Context(), strings.NewReader("Registry,Assignment,Organization Name\n"))

		require.ErrorIs(t, err, oui.ErrEmptyRegistry)
	})

	t.Run("an empty input is an error", func(t *testing.T) {
		_, err := oui.Parse(t.Context(), strings.NewReader(""))

		require.Error(t, err)
	})

	t.Run("malformed CSV is an error", func(t *testing.T) {
		_, err := oui.Parse(t.Context(), strings.NewReader("Registry,Assignment\nMA-L,\"unterminated\n"))

		require.Error(t, err)
	})
}

func TestLoadTable(t *testing.T) {
	t.Run("falls back to the embedded registry", func(t *testing.T) {
		table, err := oui.LoadTable(
			t.Context(),
			&config.OUIConfig{CachePath: filepath.Join(t.TempDir(), "missing.csv")},
		)

		require.NoError(t, err)
		require.Greater(t, len(table.ByOUI), 10000)
	})

	t.Run("prefers the cached registry", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "oui.csv")
		require.NoError(t, os.WriteFile(path, []byte(registry), 0o600))

		table, err := oui.LoadTable(t.Context(), &config.OUIConfig{CachePath: path})

		require.NoError(t, err)
		require.Len(t, table.ByOUI, 2)
	})

	t.Run("an unreadable cache is an error", func(t *testing.T) {
		_, err := oui.LoadTable(t.Context(), &config.OUIConfig{CachePath: t.TempDir()})

		require.Error(t, err)
	})
}

func TestUpdaterUpdate(t *testing.T) {
	newConfig := func(t *testing.T, url string) *config.OUIConfig {
		t.Helper()

		return &config.OUIConfig{
			CachePath:       filepath.Join(t.TempDir(), "lantern", "oui.csv"),
			SourceURL:       url,
			DownloadTimeout: time.Second,
		}
	}

	t.Run("downloads, validates and caches the registry", func(t *testing.T) {
		var userAgent string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userAgent = r.UserAgent()
			_, _ = w.Write([]byte(registry))
		}))
		t.Cleanup(server.Close)
		cfg := newConfig(t, server.URL)

		count, err := oui.NewUpdater(t.Context(), cfg).Update(t.Context())

		require.NoError(t, err)
		require.Equal(t, 2, count)
		require.Contains(t, userAgent, "lantern")
		cached, err := os.ReadFile(cfg.CachePath)
		require.NoError(t, err)
		require.Equal(t, registry, string(cached))
	})

	t.Run("an invalid download leaves the cache untouched", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html>blocked</html>"))
		}))
		t.Cleanup(server.Close)
		cfg := newConfig(t, server.URL)

		_, err := oui.NewUpdater(t.Context(), cfg).Update(t.Context())

		require.Error(t, err)
		require.NoFileExists(t, cfg.CachePath)
	})

	t.Run("a non-200 response is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		t.Cleanup(server.Close)

		_, err := oui.NewUpdater(t.Context(), newConfig(t, server.URL)).Update(t.Context())

		require.ErrorContains(t, err, "418")
	})

	t.Run("an unreachable server is an error", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		server.Close()

		_, err := oui.NewUpdater(t.Context(), newConfig(t, server.URL)).Update(t.Context())

		require.Error(t, err)
	})

	t.Run("an invalid URL is an error", func(t *testing.T) {
		_, err := oui.NewUpdater(t.Context(), newConfig(t, "://bad")).Update(t.Context())

		require.Error(t, err)
	})

	t.Run("a truncated download is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100000")
			_, _ = w.Write([]byte(registry))
		}))
		t.Cleanup(server.Close)

		_, err := oui.NewUpdater(t.Context(), newConfig(t, server.URL)).Update(t.Context())

		require.Error(t, err)
	})

	t.Run("a cache path taken by a directory is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(registry))
		}))
		t.Cleanup(server.Close)
		cfg := newConfig(t, server.URL)
		require.NoError(t, os.MkdirAll(filepath.Join(cfg.CachePath, "taken"), 0o750))

		_, err := oui.NewUpdater(t.Context(), cfg).Update(t.Context())

		require.Error(t, err)
	})

	t.Run("an unwritable cache dir is an error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(registry))
		}))
		t.Cleanup(server.Close)
		blocker := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(blocker, nil, 0o600))
		cfg := newConfig(t, server.URL)
		cfg.CachePath = filepath.Join(blocker, "oui.csv")

		_, err := oui.NewUpdater(t.Context(), cfg).Update(t.Context())

		require.Error(t, err)
	})
}
