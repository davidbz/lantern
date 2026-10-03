package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/davidbz/lantern/internal/domain"
)

const (
	scoreCellWidth = 12
	// Rows the table spends on its header and the border under it.
	tableHeaderLines = 2
	noScanYet        = "--:--:--"
)

const privilegesHint = "Active ARP sweep disabled (needs raw sockets): run with sudo or " +
	"`sudo setcap cap_net_raw+ep $(command -v lantern)`. Showing the neighbor cache and mDNS only."

// header is the logo beside the scoreboard, or a one-line wordmark above it when the window is too narrow.
func header(m Model) string {
	board := lipgloss.JoinVertical(lipgloss.Left, scoreboard(m), status(m))
	if m.width < logoWidth+logoGap+lipgloss.Width(board) {
		return lipgloss.JoinVertical(lipgloss.Left, paint(wordmark, m.frame), board)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, logo(m.frame), strings.Repeat(" ", logoGap), board)
}

func scoreboard(m Model) string {
	lastScan := noScanYet
	if !m.lastScan.IsZero() {
		lastScan = m.lastScan.Format(timeLayout)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top,
		scoreCell("DEVICES", fmt.Sprintf("%04d", len(m.devices))),
		scoreCell("HI-SCORE", fmt.Sprintf("%04d", m.hiScore)),
		scoreCell("STAGE", fmt.Sprintf("%02d", stage(m))),
		scoreCell("LAST SCAN", lastScan),
	)
}

func scoreCell(label, value string) string {
	return lipgloss.NewStyle().Width(scoreCellWidth).
		Render(bold(colorMuted).Render(label) + "\n" + bold(colorYellow).Render(value))
}

// stage is the scan being played: the running one while scanning, otherwise the last one finished.
func stage(m Model) int {
	if m.scanning {
		return m.generation + 1
	}

	return m.generation
}

func status(m Model) string {
	if m.scanning {
		return scanner(m.frame) + " " + blink(bold(colorPink).Render("SCANNING"), m.frame)
	}

	return bold(colorGreen).Render("● READY") + fg(colorMuted).Render("  NEXT WAVE IN ") +
		bold(colorCyan).Render(countdown(m).String())
}

// scanner is a bar with a lit block bouncing from end to end.
func scanner(frame int) string {
	pos := frame % scannerTrip
	if pos > scannerSpan {
		pos = scannerTrip - pos
	}

	return fg(colorDim).Render(strings.Repeat("▱", pos)) +
		fg(colorPink).Render(strings.Repeat("▰", scannerGlow)) +
		fg(colorDim).Render(strings.Repeat("▱", scannerSpan-pos))
}

// countdown is the time until the next automatic scan, as of the last animation frame.
func countdown(m Model) time.Duration {
	if m.now.IsZero() {
		return m.refresh
	}
	remaining := m.lastScan.Add(m.refresh).Sub(m.now)

	return min(max(remaining, 0), m.refresh).Round(time.Second)
}

// rule is a full-width neon line; the gradient flows along it with the animation.
func rule(m Model) string {
	return paint(strings.Repeat("━", m.width), m.frame)
}

// banners lists the scan error and each distinct warning.
func banners(m Model) []string {
	line := lipgloss.NewStyle().Width(m.width)

	var lines []string
	if m.err != nil {
		lines = append(
			lines,
			line.Render(keycap("TILT", colorRed)+" "+fg(colorRed).Render("Scan failed: "+m.err.Error())),
		)
	}
	for _, text := range warningTexts(m.warnings) {
		lines = append(lines, line.Render(keycap("CAUTION", colorOrange)+" "+fg(colorOrange).Render(text)))
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

func footer(m Model) string {
	keys := [][2]string{{"↑↓", "MOVE"}, {"ENTER", "INSPECT"}, {"S", "SORT"}, {"R", "RESCAN"}}
	if m.details {
		keys = [][2]string{{"↑↓", "BROWSE"}, {"ESC", "BACK"}, {"R", "RESCAN"}}
	}

	buttons := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		buttons = append(buttons, keycap(key[0], colorCyan)+" "+bold(colorMuted).Render(key[1]))
	}
	buttons = append(buttons, keycap("Q", colorRed)+" "+bold(colorMuted).Render("QUIT"))

	return strings.Join(buttons, "  ")
}

// splash fills the table's space while there is nothing to list.
func splash(m Model) string {
	lines := []string{
		blink(bold(colorYellow).Render("★  GET READY  ★"), m.frame),
		"",
		scanner(m.frame),
		"",
		fg(colorMuted).Render("SWEEPING THE NETWORK FOR DEVICES"),
	}
	if !m.scanning {
		lines = []string{
			bold(colorPink).Render("NO DEVICES FOUND"),
			"",
			blink(bold(colorCyan).Render("PRESS R TO TRY AGAIN"), m.frame),
		}
	}

	return lipgloss.Place(m.width, playfieldHeight(m), lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, lines...))
}

// playfieldHeight is the height of the table, which the splash and the details card fill in its place.
func playfieldHeight(m Model) int {
	return m.table.Height() + tableHeaderLines
}
