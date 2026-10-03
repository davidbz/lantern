// Command lantern discovers the devices on the local network and shows them in a terminal UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/dig"

	"github.com/davidbz/lantern/internal/app"
	"github.com/davidbz/lantern/internal/arpscan"
	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/discovery"
	"github.com/davidbz/lantern/internal/logging"
	"github.com/davidbz/lantern/internal/mdns"
	"github.com/davidbz/lantern/internal/neighbor"
	"github.com/davidbz/lantern/internal/netif"
	"github.com/davidbz/lantern/internal/oui"
	"github.com/davidbz/lantern/internal/tui"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	stop()

	// -h printed usage; a signal stopped the UI: both are normal exits.
	if errors.Is(err, flag.ErrHelp) || errors.Is(err, context.Canceled) {
		return
	}
	if err != nil {
		log.Fatalf("lantern: %v", err)
	}
}

func run(ctx context.Context, args []string) error {
	container, err := buildContainer(ctx, args)
	if err != nil {
		return err
	}

	var cli *config.CLIConfig
	if err := container.Invoke(func(c *config.CLIConfig) { cli = c }); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cli.Version {
		if _, err := fmt.Fprintln(os.Stdout, "lantern", version); err != nil {
			return fmt.Errorf("failed to print version: %w", err)
		}
		return nil
	}

	var entrypoint any = func(deps app.BrowseDeps) error { return app.Browse(ctx, deps, os.Stdin, os.Stdout) }
	if cli.UpdateOUI {
		entrypoint = func(deps app.UpdateDeps) error { return app.UpdateVendors(ctx, deps, os.Stdout) }
	}

	if err := container.Invoke(entrypoint); err != nil {
		return fmt.Errorf("lantern stopped: %w", err)
	}

	return nil
}

func buildContainer(ctx context.Context, args []string) (*dig.Container, error) {
	container := dig.New()
	source := []dig.ProvideOption{dig.Group(discovery.SourceGroup), dig.As(new(discovery.Source))}

	providers := []struct {
		constructor any
		opts        []dig.ProvideOption
	}{
		{func() context.Context { return ctx }, nil},
		{func() (*config.Config, error) { return config.Load(ctx, args, os.Stderr) }, nil},
		{config.ParseDependenciesConfig, nil},
		{logging.New, nil},
		{oui.LoadTable, nil},
		{oui.NewUpdater, []dig.ProvideOption{dig.As(new(app.VendorUpdater))}},
		{netif.NewLister, []dig.ProvideOption{dig.As(new(discovery.InterfaceLister))}},
		{arpscan.NewScanner, source},
		{neighbor.NewReader, source},
		{mdns.NewZeroconfBrowser, []dig.ProvideOption{dig.As(new(mdns.Browser))}},
		{mdns.NewDiscoverer, source},
		{discovery.NewService, []dig.ProvideOption{dig.As(new(tui.Scanner))}},
	}
	for _, provider := range providers {
		if err := container.Provide(provider.constructor, provider.opts...); err != nil {
			return nil, fmt.Errorf("failed to register constructor: %w", err)
		}
	}

	return container, nil
}
