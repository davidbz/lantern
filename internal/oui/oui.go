// Package oui loads the IEEE MA-L registry that maps MAC prefixes to vendors.
package oui

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
)

// IEEE CSV layout: Registry,Assignment,Organization Name,Organization Address.
const (
	assignmentColumn   = 1
	organizationColumn = 2
	minColumns         = 3
	assignmentHexLen   = 6
)

var ErrEmptyRegistry = errors.New("vendor registry has no entries")

// embeddedRegistry is the IEEE registry at build time (gzipped); refresh it with `make oui`.
//
//go:embed data/oui.csv.gz
var embeddedRegistry []byte

// LoadTable prefers the registry downloaded by --update-oui and falls back to the embedded copy.
func LoadTable(ctx context.Context, cfg *config.OUIConfig) (domain.VendorTable, error) {
	cached, err := os.ReadFile(cfg.CachePath)
	if err == nil {
		return Parse(ctx, bytes.NewReader(cached))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return domain.VendorTable{}, fmt.Errorf("failed to read cached vendor registry: %w", err)
	}

	reader, err := gzip.NewReader(bytes.NewReader(embeddedRegistry))
	if err != nil {
		return domain.VendorTable{}, fmt.Errorf("failed to open embedded vendor registry: %w", err)
	}

	return Parse(ctx, reader)
}

// Parse reads an IEEE registry CSV. Rows whose assignment is not a 24-bit OUI are skipped.
func Parse(_ context.Context, r io.Reader) (domain.VendorTable, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true

	if _, err := reader.Read(); err != nil {
		return domain.VendorTable{}, fmt.Errorf("failed to read vendor registry header: %w", err)
	}

	table := domain.VendorTable{ByOUI: make(map[domain.OUI]string)}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return domain.VendorTable{}, fmt.Errorf("failed to read vendor registry: %w", err)
		}

		prefix, ok := parseAssignment(record)
		if !ok {
			continue
		}
		table.ByOUI[prefix] = strings.TrimSpace(record[organizationColumn])
	}

	if len(table.ByOUI) == 0 {
		return domain.VendorTable{}, ErrEmptyRegistry
	}

	return table, nil
}

func parseAssignment(record []string) (domain.OUI, bool) {
	if len(record) < minColumns || len(record[assignmentColumn]) != assignmentHexLen {
		return domain.OUI{}, false
	}

	var prefix domain.OUI
	if _, err := hex.Decode(prefix[:], []byte(record[assignmentColumn])); err != nil {
		return domain.OUI{}, false
	}

	return prefix, true
}
