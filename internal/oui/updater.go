package oui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/logging"
)

const (
	cacheDirMode = 0o750
	userAgent    = "lantern (+https://github.com/davidbz/lantern)"
)

type Updater struct {
	cfg *config.OUIConfig
}

func NewUpdater(_ context.Context, cfg *config.OUIConfig) *Updater {
	return &Updater{cfg: cfg}
}

// Update downloads and validates the registry, then atomically replaces the cached copy.
// It returns the number of vendors in the new registry.
func (u *Updater) Update(ctx context.Context) (int, error) {
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "downloading vendor registry", "url", u.cfg.SourceURL)

	data, err := u.download(ctx)
	if err != nil {
		return 0, err
	}

	table, err := Parse(ctx, bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("downloaded vendor registry is invalid: %w", err)
	}

	if err := writeAtomically(u.cfg.CachePath, data); err != nil {
		return 0, err
	}

	logger.InfoContext(ctx, "vendor registry updated", "path", u.cfg.CachePath, "vendors", len(table.ByOUI))

	return len(table.ByOUI), nil
}

func (u *Updater) download(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, u.cfg.DownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.cfg.SourceURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to build registry request: %w", err)
	}
	// The IEEE server rejects requests without a descriptive User-Agent.
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download vendor registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download vendor registry: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read vendor registry: %w", err)
	}

	return data, nil
}

func writeAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, cacheDirMode); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return fmt.Errorf("failed to write vendor registry: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("failed to replace vendor registry: %w", err)
	}

	return nil
}
