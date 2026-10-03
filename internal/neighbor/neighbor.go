// Package neighbor reads the kernel's neighbor (ARP) cache, which needs no privileges.
package neighbor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
)

// /proc/net/arp columns: IP address, HW type, Flags, HW address, Mask, Device.
const (
	ipColumn     = 0
	flagsColumn  = 2
	macColumn    = 3
	deviceColumn = 5
	columnCount  = 6
	// incompleteFlags marks an entry whose resolution never completed.
	incompleteFlags = "0x0"
)

type Reader struct {
	cfg *config.ScanConfig
}

func NewReader(_ context.Context, cfg *config.ScanConfig) *Reader {
	return &Reader{cfg: cfg}
}

func (r *Reader) Discover(ctx context.Context, ifaces []domain.Interface) ([]domain.Observation, error) {
	file, err := os.Open(r.cfg.NeighborTablePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("neighbor table %s: %w", r.cfg.NeighborTablePath, domain.ErrSourceUnavailable)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open neighbor table: %w", err)
	}
	defer file.Close()

	return ParseTable(ctx, file, ifaces, time.Now())
}

// ParseTable converts complete neighbor entries on the scanned interfaces into observations.
func ParseTable(
	ctx context.Context,
	r io.Reader,
	ifaces []domain.Interface,
	now time.Time,
) ([]domain.Observation, error) {
	scanner := bufio.NewScanner(r)
	scanner.Scan() // header

	var observations []domain.Observation
	for scanner.Scan() {
		obs, ok := parseLine(ctx, scanner.Text(), ifaces, now)
		if !ok {
			continue
		}
		observations = append(observations, obs)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read neighbor table: %w", err)
	}

	return observations, nil
}

func parseLine(ctx context.Context, line string, ifaces []domain.Interface, now time.Time) (domain.Observation, bool) {
	fields := strings.Fields(line)
	if len(fields) != columnCount || fields[flagsColumn] == incompleteFlags || !scanned(fields[deviceColumn], ifaces) {
		return domain.Observation{}, false
	}

	ip, err := netip.ParseAddr(fields[ipColumn])
	if err != nil {
		return domain.Observation{}, false
	}
	mac, err := domain.ParseMAC(ctx, fields[macColumn])
	if err != nil || domain.IsZeroMAC(ctx, mac) {
		return domain.Observation{}, false
	}

	return domain.Observation{
		Source:   domain.SourceNeighbor,
		IP:       ip,
		MAC:      mac,
		Hostname: "",
		Services: nil,
		SeenAt:   now,
	}, true
}

func scanned(device string, ifaces []domain.Interface) bool {
	return slices.ContainsFunc(ifaces, func(iface domain.Interface) bool { return iface.Name == device })
}
