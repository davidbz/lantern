package tui

import (
	"context"
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
	rankWidth        = 5
	ipWidth          = 18
	macWidth         = 17
	vendorWidth      = 26
	hostnameWidth    = 26
	minServicesWidth = 16
	sourcesWidth     = 16
	lastSeenWidth    = 11
	detailLabelWidth = 12
	columnPadding    = 2 // cells the table adds around each column
	columnCount      = 8

	minTableHeight = 3

	timeLayout  = "15:04:05"
	placeholder = "-"
	listSep     = ", "
	sortMarker  = " ▼"
	newMarker   = "★"
	unknownName = "UNKNOWN"

	// Ordinal suffixes: 1ST, 2ND, 3RD, then TH, except 11TH to 13TH.
	ordinalBase    = 10
	ordinalCentury = 100
	firstTeen      = 11
	lastTeen       = 13
	first          = 1
	second         = 2
	third          = 3

	cardPaddingY = 0
	cardPaddingX = 3
)

func newTable(ctx context.Context) table.Model {
	return table.New(table.WithColumns(columns(ctx, 0, domain.SortByIP)), table.WithFocused(true),
		table.WithStyles(tableStyles()))
}

// columns sizes the table for width and marks the column it is sorted by.
func columns(ctx context.Context, width int, sortKey domain.SortKey) []table.Column {
	fixed := rankWidth + ipWidth + macWidth + vendorWidth + hostnameWidth + sourcesWidth + lastSeenWidth
	services := max(width-fixed-columnPadding*columnCount, minServicesWidth)

	cols := []table.Column{
		{Title: "RANK", Width: rankWidth},
		{Title: "IP", Width: ipWidth},
		{Title: "MAC", Width: macWidth},
		{Title: "VENDOR", Width: vendorWidth},
		{Title: "HOSTNAME", Width: hostnameWidth},
		{Title: "SERVICES", Width: services},
		{Title: "SEEN BY", Width: sourcesWidth},
		{Title: "LAST SEEN", Width: lastSeenWidth},
	}
	sorted := strings.ToUpper(domain.SortKeyName(ctx, sortKey))
	for i := range cols {
		if cols[i].Title == sorted {
			cols[i].Title += sortMarker
		}
	}

	return cols
}

// rows lists the devices; those first seen after since are starred as new arrivals.
func rows(ctx context.Context, devices []domain.Device, since time.Time) []table.Row {
	result := make([]table.Row, 0, len(devices))
	for i := range devices {
		device := &devices[i]
		rank := ordinal(i + 1)
		if !since.IsZero() && device.FirstSeen.After(since) {
			rank += newMarker
		}
		result = append(result, table.Row{
			rank,
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
	sections := slices.Concat(chromeTop(m), []string{playfield(ctx, m)}, chromeBottom(m))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// chromeTop is everything above the playfield: header, rule and banners.
func chromeTop(m Model) []string {
	return append([]string{header(m), rule(m)}, banners(m)...)
}

// chromeBottom is everything below the playfield: rule and footer.
func chromeBottom(m Model) []string {
	return []string{rule(m), footer(m)}
}

// chromeHeight is the number of lines around the table, so the table fills the rest of the window.
func chromeHeight(m Model) int {
	return lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, slices.Concat(chromeTop(m), chromeBottom(m))...))
}

func playfield(ctx context.Context, m Model) string {
	device, ok := m.selected()
	if m.details && ok {
		return details(ctx, m, device)
	}
	if len(m.devices) == 0 {
		return splash(m)
	}

	return m.table.View()
}

// details is the selected device's player card.
func details(ctx context.Context, m Model, device domain.Device) string {
	label := bold(colorCyan).Width(detailLabelWidth)
	fields := [][2]string{
		{"IPS", orPlaceholder(strings.Join(addrStrings(device.IPs), listSep))},
		{"MAC", orPlaceholder(domain.FormatMAC(ctx, device.MAC))},
		{"VENDOR", orPlaceholder(device.Vendor)},
		{"HOSTNAME", orPlaceholder(device.Hostname)},
		{"SERVICES", chips(device.Services)},
		{"SEEN BY", lamps(ctx, device.Sources)},
		{"FIRST SEEN", device.FirstSeen.Format(time.DateTime)},
		{"LAST SEEN", device.LastSeen.Format(time.DateTime)},
	}

	lines := []string{
		bold(colorMuted).Render(fmt.Sprintf("PLAYER %02d OF %02d", m.table.Cursor()+1, len(m.devices))),
		bold(colorYellow).Render(displayName(ctx, device)),
		fg(colorPurple).Render(orPlaceholder(device.Vendor)),
		"",
	}
	for _, field := range fields {
		lines = append(lines, label.Render(field[0])+field[1])
	}

	card := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(colorPink).
		Padding(cardPaddingY, cardPaddingX).Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, playfieldHeight(m), lipgloss.Center, lipgloss.Top, card)
}

// displayName is the most human name known for a device.
func displayName(ctx context.Context, device domain.Device) string {
	if device.Hostname != "" {
		return device.Hostname
	}
	if len(device.IPs) > 0 {
		return device.IPs[0].String()
	}
	if mac := domain.FormatMAC(ctx, device.MAC); mac != "" {
		return mac
	}

	return unknownName
}

// chips renders each service as a badge.
func chips(services []string) string {
	if len(services) == 0 {
		return placeholder
	}

	badges := make([]string, 0, len(services))
	for _, service := range services {
		badges = append(badges, bold(colorGreen).Render("["+service+"]"))
	}

	return strings.Join(badges, " ")
}

// lamps shows every source, lit when it saw the device.
func lamps(ctx context.Context, sources domain.SourceKind) string {
	seen := domain.SourceNames(ctx, sources)
	all := domain.SourceNames(ctx, domain.SourceARP|domain.SourceNeighbor|domain.SourceMDNS)

	result := make([]string, 0, len(all))
	for _, name := range all {
		lamp := fg(colorDim).Render("◇ " + strings.ToUpper(name))
		if slices.Contains(seen, name) {
			lamp = bold(colorGreen).Render("◆ " + strings.ToUpper(name))
		}
		result = append(result, lamp)
	}

	return strings.Join(result, "  ")
}

// ordinal renders a leaderboard rank: 1ST, 2ND, 3RD, 4TH, ...
func ordinal(rank int) string {
	suffix := "TH"
	if teen := rank % ordinalCentury; teen < firstTeen || teen > lastTeen {
		switch rank % ordinalBase {
		case first:
			suffix = "ST"
		case second:
			suffix = "ND"
		case third:
			suffix = "RD"
		}
	}

	return fmt.Sprintf("%d%s", rank, suffix)
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
