package tui

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"

	"github.com/davidbz/lantern/internal/domain"
)

// Column widths in cells. Services takes whatever width is left, but never less than its minimum.
const (
	ipWidth          = 18
	macWidth         = 17
	vendorWidth      = 26
	hostnameWidth    = 26
	minServicesWidth = 16
	sourcesWidth     = 16
	lastSeenWidth    = 9
	detailLabelWidth = 12
	columnPadding    = 2 // cells the table adds around each column
	columnCount      = 7

	minTableHeight = 3
	// Lines around the table: title, footer and the table header with its border.
	fixedChromeLines = 4

	timeLayout  = "15:04:05"
	placeholder = "-"
	listSep     = ", "

	colorAccent  = "39"
	colorWarning = "214"
	colorError   = "196"
	colorMuted   = "245"
)

const privilegesHint = "Active ARP sweep disabled (needs raw sockets): run with sudo or " +
	"`sudo setcap cap_net_raw+ep $(command -v lantern)`. Showing the neighbor cache and mDNS only."

func newTable() table.Model {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).BorderStyle(lipgloss.NormalBorder()).BorderBottom(true)
	styles.Selected = styles.Selected.Foreground(lipgloss.Color(colorAccent)).Bold(true)

	return table.New(table.WithColumns(columns(0)), table.WithFocused(true), table.WithStyles(styles))
}

func columns(width int) []table.Column {
	fixed := ipWidth + macWidth + vendorWidth + hostnameWidth + sourcesWidth + lastSeenWidth
	services := max(width-fixed-columnPadding*columnCount, minServicesWidth)

	return []table.Column{
		{Title: "IP", Width: ipWidth},
		{Title: "MAC", Width: macWidth},
		{Title: "Vendor", Width: vendorWidth},
		{Title: "Hostname", Width: hostnameWidth},
		{Title: "Services", Width: services},
		{Title: "Seen by", Width: sourcesWidth},
		{Title: "Last seen", Width: lastSeenWidth},
	}
}

func rows(ctx context.Context, devices []domain.Device) []table.Row {
	result := make([]table.Row, 0, len(devices))
	for i := range devices {
		device := &devices[i]
		result = append(result, table.Row{
			ipSummary(device.IPs),
			orPlaceholder(domain.FormatMAC(ctx, device.MAC)),
			orPlaceholder(device.Vendor),
			orPlaceholder(device.Hostname),
			orPlaceholder(strings.Join(device.Services, listSep)),
			strings.Join(domain.SourceNames(ctx, device.Sources), listSep),
			device.LastSeen.Format(timeLayout),
		})
	}

	return result
}

func render(ctx context.Context, m Model) string {
	sections := []string{title(ctx, m)}
	sections = append(sections, banners(m)...)

	device, ok := m.selected()
	if m.details && ok {
		sections = append(sections, details(ctx, device))
	}
	if !m.details || !ok {
		sections = append(sections, m.table.View())
	}

	sections = append(sections, footer())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func title(ctx context.Context, m Model) string {
	name := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent)).Render("lantern")
	status := deviceCount(len(m.devices)) + " · sorted by " + domain.SortKeyName(ctx, m.sortKey)
	if m.scanning {
		return name + "  " + status + " · scanning…"
	}

	return name + "  " + status + " · last scan " + m.lastScan.Format(timeLayout)
}

// banners lists the scan error and each distinct warning, one line each.
func banners(m Model) []string {
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarning)).Width(m.width)
	fail := lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Width(m.width)

	var lines []string
	if m.err != nil {
		lines = append(lines, fail.Render("✗ Scan failed: "+m.err.Error()))
	}
	for _, text := range warningTexts(m.warnings) {
		lines = append(lines, warn.Render("! "+text))
	}

	return lines
}

func warningTexts(warnings []error) []string {
	var texts []string
	for _, warning := range warnings {
		text := warning.Error()
		if errors.Is(warning, domain.ErrInsufficientPrivileges) {
			text = privilegesHint
		}
		if slices.Contains(texts, text) {
			continue
		}
		texts = append(texts, text)
	}

	return texts
}

func details(ctx context.Context, device domain.Device) string {
	label := lipgloss.NewStyle().Bold(true).Width(detailLabelWidth)
	fields := [][2]string{
		{"IPs", strings.Join(addrStrings(device.IPs), listSep)},
		{"MAC", orPlaceholder(domain.FormatMAC(ctx, device.MAC))},
		{"Vendor", orPlaceholder(device.Vendor)},
		{"Hostname", orPlaceholder(device.Hostname)},
		{"Services", orPlaceholder(strings.Join(device.Services, listSep))},
		{"Seen by", strings.Join(domain.SourceNames(ctx, device.Sources), listSep)},
		{"First seen", device.FirstSeen.Format(time.DateTime)},
		{"Last seen", device.LastSeen.Format(time.DateTime)},
	}

	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		lines = append(lines, label.Render(field[0])+field[1])
	}

	return lipgloss.NewStyle().Padding(1, columnPadding).Render(strings.Join(lines, "\n"))
}

func footer() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted)).
		Render("↑/↓ move · enter details · s sort · r rescan · q quit")
}

// chromeHeight is the number of lines around the table, so the table fills the rest of the window.
func chromeHeight(m Model) int {
	return fixedChromeLines + len(banners(m))
}

func deviceCount(count int) string {
	if count == 1 {
		return "1 device"
	}

	return fmt.Sprintf("%d devices", count)
}

func ipSummary(ips []netip.Addr) string {
	if len(ips) == 0 {
		return placeholder
	}
	if len(ips) == 1 {
		return ips[0].String()
	}

	return fmt.Sprintf("%s +%d", ips[0], len(ips)-1)
}

func addrStrings(ips []netip.Addr) []string {
	texts := make([]string, 0, len(ips))
	for _, ip := range ips {
		texts = append(texts, ip.String())
	}

	return texts
}

func orPlaceholder(text string) string {
	if text == "" {
		return placeholder
	}

	return text
}
