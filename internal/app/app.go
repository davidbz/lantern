// Package app holds lantern's entry points: browsing the network and updating the vendor registry.
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"go.uber.org/dig"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/logging"
	"github.com/davidbz/lantern/internal/tracing"
	"github.com/davidbz/lantern/internal/tui"
)

type VendorUpdater interface {
	Update(ctx context.Context) (int, error)
}

type BrowseDeps struct {
	dig.In

	Logger  *slog.Logger
	Scanner tui.Scanner
	UI      *config.UIConfig
}

type UpdateDeps struct {
	dig.In

	Logger  *slog.Logger
	Updater VendorUpdater
	OUI     *config.OUIConfig
}

// Browse runs the interactive device browser on the in/out terminal.
func Browse(ctx context.Context, deps BrowseDeps, in io.Reader, out io.Writer) error {
	ctx = logging.WithLogger(tracing.WithNewTraceID(ctx), deps.Logger)

	if err := tui.Run(ctx, deps.Scanner, deps.UI, in, out); err != nil {
		return fmt.Errorf("failed to browse devices: %w", err)
	}

	return nil
}

// UpdateVendors downloads the latest vendor registry and reports the result on out.
func UpdateVendors(ctx context.Context, deps UpdateDeps, out io.Writer) error {
	ctx = logging.WithLogger(tracing.WithNewTraceID(ctx), deps.Logger)

	count, err := deps.Updater.Update(ctx)
	if err != nil {
		return fmt.Errorf("failed to update vendor registry: %w", err)
	}

	if _, err := fmt.Fprintf(out, "Saved %d vendors to %s\n", count, deps.OUI.CachePath); err != nil {
		return fmt.Errorf("failed to report update: %w", err)
	}

	return nil
}
