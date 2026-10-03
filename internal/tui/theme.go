package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// Synthwave palette. lipgloss downsamples these on terminals without true color.
const (
	colorPink   = lipgloss.Color("#FF2E97")
	colorPurple = lipgloss.Color("#B967FF")
	colorCyan   = lipgloss.Color("#01CDFE")
	colorGreen  = lipgloss.Color("#05FFA1")
	colorYellow = lipgloss.Color("#FFE600")
	colorOrange = lipgloss.Color("#FF9F1C")
	colorRed    = lipgloss.Color("#FF3864")
	colorInk    = lipgloss.Color("#0D0221") // text on bright backgrounds
	colorMuted  = lipgloss.Color("#8B7FB8")
	colorDim    = lipgloss.Color("#3D3363")
)

const (
	frameInterval = 100 * time.Millisecond
	blinkFrames   = 5 // frames per blink phase, so text blinks once a second

	scannerWidth = 12 // cells in the scanning bar
	scannerGlow  = 3  // lit cells sweeping across it
	scannerSpan  = scannerWidth - scannerGlow
	scannerTrip  = 2 * scannerSpan // frames for the lit block to go there and back

	// The block-letter logo; every row is logoWidth cells wide.
	logoTop    = "█   ▄▀▄ █▄ █ ▀█▀ █▀▀ █▀▄ █▄ █"
	logoMiddle = "█   █▀█ █ ▀█  █  █▀▀ █▀▄ █ ▀█"
	logoBottom = "█▄▄ █ █ █  █  █  █▄▄ █ █ █  █"
	logoWidth  = 29
	logoGap    = 4
	wordmark   = "LANTERN"
)

// gradient is the color cycle that scrolls across the logo and the rules. It runs there and back so it loops
// without a seam.
func gradient() []lipgloss.Color {
	return []lipgloss.Color{
		"#FF2E97", "#F23BB5", "#D94FE0", "#B967FF", "#8A7CFF", "#5B9DFF",
		"#01CDFE", "#5B9DFF", "#8A7CFF", "#B967FF", "#D94FE0", "#F23BB5",
	}
}

// paint colors each rune of text by its column; a growing offset scrolls the gradient.
func paint(text string, offset int) string {
	stops := gradient()

	var builder strings.Builder
	for col, char := range []rune(text) {
		if char == ' ' {
			builder.WriteRune(char)
			continue
		}
		style := lipgloss.NewStyle().Bold(true).Foreground(stops[(col+offset)%len(stops)])
		builder.WriteString(style.Render(string(char)))
	}

	return builder.String()
}

// logo renders the block letters with the gradient running diagonally.
func logo(frame int) string {
	rows := []string{logoTop, logoMiddle, logoBottom}
	for i, row := range rows {
		rows[i] = paint(row, frame+i)
	}

	return strings.Join(rows, "\n")
}

// blink shows text in the first half of each blink cycle and blanks it, keeping its width, in the second.
func blink(text string, frame int) string {
	if (frame/blinkFrames)%2 == 0 {
		return text
	}

	return strings.Repeat(" ", lipgloss.Width(text))
}

// keycap renders a key as a lit arcade button.
func keycap(key string, color lipgloss.Color) string {
	return lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(color).Padding(0, 1).Render(key)
}

func fg(color lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(color)
}

func bold(color lipgloss.Color) lipgloss.Style {
	return fg(color).Bold(true)
}

// tableStyles leaves cells unstyled: the selection bar's background would otherwise stop at the first cell reset.
func tableStyles() table.Styles {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).Foreground(colorCyan).
		BorderStyle(lipgloss.ThickBorder()).BorderBottom(true).BorderForeground(colorPurple)
	styles.Selected = lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorPink)

	return styles
}
