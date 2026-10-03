package config_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/config"
)

func TestLoad(t *testing.T) {
	t.Run("applies defaults", func(t *testing.T) {
		t.Chdir(t.TempDir())
		cacheDir := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", cacheDir)

		cfg, err := config.Load(t.Context(), nil, &bytes.Buffer{})

		require.NoError(t, err)
		require.False(t, cfg.CLI.UpdateOUI)
		require.Equal(t, 8*time.Second, cfg.Scan.Timeout)
		require.Equal(t, 22, cfg.Scan.ARPMinPrefixBits)
		require.Contains(t, cfg.Scan.MDNSServices, "_googlecast._tcp")
		require.Equal(t, filepath.Join(cacheDir, "lantern", "oui.csv"), cfg.OUI.CachePath)
		require.Equal(t, 30*time.Second, cfg.UI.RefreshInterval)
		require.Empty(t, cfg.Log.Path)
	})

	t.Run("reads flags and environment", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("LANTERN_OUI_CACHE", "/tmp/oui.csv")
		t.Setenv("LANTERN_MDNS_SERVICES", "_ipp._tcp,_ssh._tcp")

		cfg, err := config.Load(t.Context(), []string{"-update-oui", "-version"}, &bytes.Buffer{})

		require.NoError(t, err)
		require.True(t, cfg.CLI.UpdateOUI)
		require.True(t, cfg.CLI.Version)
		require.Equal(t, "/tmp/oui.csv", cfg.OUI.CachePath)
		require.Equal(t, []string{"_ipp._tcp", "_ssh._tcp"}, cfg.Scan.MDNSServices)
	})

	t.Run("-h prints usage", func(t *testing.T) {
		var usage bytes.Buffer

		_, err := config.Load(t.Context(), []string{"-h"}, &usage)

		require.ErrorIs(t, err, flag.ErrHelp)
		require.Contains(t, usage.String(), "-update-oui")
	})

	t.Run("invalid values are errors", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("LANTERN_SCAN_TIMEOUT", "soon")

		_, err := config.Load(t.Context(), nil, &bytes.Buffer{})

		require.Error(t, err)
	})

	t.Run("an unreadable .env is an error", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".env"), 0o700))

		_, err := config.Load(t.Context(), nil, &bytes.Buffer{})

		require.Error(t, err)
	})

	t.Run("a missing cache dir is an error", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("XDG_CACHE_HOME", "")
		t.Setenv("HOME", "")

		_, err := config.Load(t.Context(), nil, &bytes.Buffer{})

		require.Error(t, err)
	})
}

func TestParseDependenciesConfig(t *testing.T) {
	cfg := &config.Config{}

	deps := config.ParseDependenciesConfig(t.Context(), cfg)

	require.Same(t, &cfg.Scan, deps.ScanConfig)
	require.Same(t, &cfg.OUI, deps.OUIConfig)
	require.Same(t, &cfg.UI, deps.UIConfig)
	require.Same(t, &cfg.Log, deps.LogConfig)
	require.Same(t, &cfg.CLI, deps.CLIConfig)
}
