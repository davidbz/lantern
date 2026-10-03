package tui

import (
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/logging"
	"github.com/davidbz/lantern/internal/tracing"
)

type Scanner interface {
	Scan(ctx context.Context, prev domain.Inventory) (domain.ScanResult, error)
}

// Run shows the TUI on in/out until the user quits or ctx is cancelled; cancellation surfaces as an error wrapping
// ctx.Err().
func Run(ctx context.Context, scanner Scanner, cfg *config.UIConfig, in io.Reader, out io.Writer) error {
	// Signals arrive as ctx cancellation. Bubble Tea's own handler would race it and can block shutdown forever.
	program := tea.NewProgram(NewModel(ctx, BindScanner(ctx, scanner), cfg),
		tea.WithAltScreen(), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out), tea.WithoutSignalHandler())

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("terminal UI stopped: %w", err)
	}

	return nil
}

// BindScanner adapts a Scanner to a ScanFunc; each scan gets its own trace ID.
func BindScanner(ctx context.Context, scanner Scanner) ScanFunc {
	return func(prev domain.Inventory) tea.Cmd {
		return func() tea.Msg {
			scanCtx := tracing.WithNewTraceID(ctx)
			result, err := scanner.Scan(scanCtx, prev)
			if err != nil {
				// The error is shown in the UI, which has no caller to return it to; log it for diagnosis.
				logging.FromContext(scanCtx).ErrorContext(scanCtx, "scan failed", "error", err)
			}

			return ScanDoneMsg{Result: result, Err: err}
		}
	}
}
