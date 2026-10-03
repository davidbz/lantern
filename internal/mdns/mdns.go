// Package mdns discovers hostnames and services announced over multicast DNS (DNS-SD).
package mdns

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
)

const (
	servicePrefix = "_"
	labelSep      = "."
)

type Record struct {
	Service  string
	HostName string
	IPv4     []netip.Addr
}

// Browser returns every instance of a service type announced until ctx is done.
type Browser interface {
	Browse(ctx context.Context, service string) ([]Record, error)
}

type Discoverer struct {
	browser Browser
	cfg     *config.ScanConfig
}

func NewDiscoverer(_ context.Context, browser Browser, cfg *config.ScanConfig) *Discoverer {
	return &Discoverer{browser: browser, cfg: cfg}
}

// Discover browses every configured service type concurrently for the configured wait.
func (d *Discoverer) Discover(ctx context.Context, ifaces []domain.Interface) ([]domain.Observation, error) {
	ctx, cancel := context.WithTimeout(ctx, d.cfg.MDNSWait)
	defer cancel()

	results := make([][]Record, len(d.cfg.MDNSServices))
	failures := make([]error, len(d.cfg.MDNSServices))
	var wg sync.WaitGroup
	for i, service := range d.cfg.MDNSServices {
		wg.Go(func() {
			results[i], failures[i] = d.browser.Browse(ctx, service)
		})
	}
	wg.Wait()

	if err := errors.Join(failures...); err != nil {
		return nil, err
	}

	now := time.Now()
	var observations []domain.Observation
	for _, records := range results {
		observations = append(observations, ToObservations(ctx, records, ifaces, now)...)
	}

	return observations, nil
}

// ToObservations keeps the addresses that belong to the scanned networks.
func ToObservations(
	ctx context.Context,
	records []Record,
	ifaces []domain.Interface,
	now time.Time,
) []domain.Observation {
	var observations []domain.Observation
	for _, record := range records {
		for _, ip := range record.IPv4 {
			if !domain.InNetworks(ctx, ip, ifaces) {
				continue
			}
			observations = append(observations, domain.Observation{
				Source:   domain.SourceMDNS,
				IP:       ip,
				MAC:      domain.MAC{},
				Hostname: strings.TrimSuffix(record.HostName, labelSep),
				Services: []string{ServiceLabel(ctx, record.Service)},
				SeenAt:   now,
			})
		}
	}

	return observations
}

// ServiceLabel shortens a DNS-SD type for display: "_ipp._tcp" becomes "ipp".
func ServiceLabel(_ context.Context, service string) string {
	name, _, _ := strings.Cut(service, labelSep)
	return strings.TrimPrefix(name, servicePrefix)
}
