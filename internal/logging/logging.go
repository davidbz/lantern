// Package logging provides the structured logger, carried in the context.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/tracing"
)

const (
	logFileMode = 0o600
	traceIDAttr = "trace_id"
)

type key int

const loggerKey key = iota

// New builds the application logger. Stdout belongs to the TUI, so logs go to a file or nowhere.
func New(ctx context.Context, cfg *config.LogConfig) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", cfg.Level, err)
	}

	if cfg.Path == "" {
		return slog.New(slog.DiscardHandler), nil
	}

	// The file stays open for the life of the process; the OS closes it on exit.
	file, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, logFileMode)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return NewWithWriter(ctx, file, level), nil
}

func NewWithWriter(_ context.Context, out io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level}))
}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext returns the context's logger, tagged with the trace ID; a discarding logger when none is set.
func FromContext(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerKey).(*slog.Logger)
	if !ok {
		logger = slog.New(slog.DiscardHandler)
	}

	traceID := tracing.TraceID(ctx)
	if traceID == "" {
		return logger
	}

	return logger.With(traceIDAttr, traceID)
}
