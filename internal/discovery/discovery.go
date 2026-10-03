// Package discovery runs every device source and merges what they observe into the inventory.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"go.uber.org/dig"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/logging"
)

const SourceGroup = "sources"

var ErrNoInterfaces = errors.New("no network interface with an IPv4 address to scan")

type Source interface {
	Discover(ctx context.Context, ifaces []domain.Interface) ([]domain.Observation, error)
}

type InterfaceLister interface {
	List(ctx context.Context) ([]domain.Interface, error)
}

type Params struct {
	dig.In

	Sources    []Source `group:"sources"`
	Interfaces InterfaceLister
	Vendors    domain.VendorTable
	Config     *config.ScanConfig
}

type Service struct {
	sources    []Source
	interfaces InterfaceLister
	vendors    domain.VendorTable
	cfg        *config.ScanConfig
}

func NewService(_ context.Context, params Params) *Service {
	return &Service{
		sources:    params.Sources,
		interfaces: params.Interfaces,
		vendors:    params.Vendors,
		cfg:        params.Config,
	}
}

// Scan runs every source concurrently and merges their observations into prev, returning the new inventory.
// Sources that cannot run here (missing privileges, unsupported OS) become warnings; any other failure fails the scan.
func (s *Service) Scan(ctx context.Context, prev domain.Inventory) (domain.ScanResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	ifaces, err := s.interfaces.List(ctx)
	if err != nil {
		return domain.ScanResult{}, fmt.Errorf("failed to list interfaces: %w", err)
	}
	if len(ifaces) == 0 {
		return domain.ScanResult{}, ErrNoInterfaces
	}

	batches := make([][]domain.Observation, len(s.sources))
	failures := make([]error, len(s.sources))
	var wg sync.WaitGroup
	for i, source := range s.sources {
		wg.Go(func() {
			batches[i], failures[i] = source.Discover(ctx, ifaces)
		})
	}
	wg.Wait()

	warnings, err := splitFailures(failures)
	if err != nil {
		return domain.ScanResult{}, err
	}

	observations := slices.Concat(batches...)
	inventory := domain.ApplyVendors(ctx, domain.Merge(ctx, prev, observations), s.vendors)

	logging.FromContext(ctx).DebugContext(ctx, "scan finished",
		"interfaces", len(ifaces), "observations", len(observations), "devices", len(inventory.Devices),
		"warnings", len(warnings))

	return domain.ScanResult{Inventory: inventory, Warnings: warnings}, nil
}

// splitFailures separates sources that are unavailable here (warnings) from real errors.
func splitFailures(failures []error) ([]error, error) {
	var warnings, errs []error
	for _, failure := range failures {
		if failure == nil {
			continue
		}
		if errors.Is(failure, domain.ErrInsufficientPrivileges) || errors.Is(failure, domain.ErrSourceUnavailable) {
			warnings = append(warnings, failure)
			continue
		}
		errs = append(errs, failure)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("discovery failed: %w", errors.Join(errs...))
	}

	return warnings, nil
}
