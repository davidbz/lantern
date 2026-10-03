package logging_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/logging"
	"github.com/davidbz/lantern/internal/tracing"
)

func TestNew(t *testing.T) {
	t.Run("writes to the configured file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "lantern.log")

		logger, err := logging.New(t.Context(), &config.LogConfig{Path: path, Level: "debug"})
		require.NoError(t, err)
		logger.DebugContext(t.Context(), "hello")

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Contains(t, string(data), "hello")
	})

	t.Run("discards logs without a file", func(t *testing.T) {
		logger, err := logging.New(t.Context(), &config.LogConfig{Level: "info"})

		require.NoError(t, err)
		require.False(t, logger.Enabled(t.Context(), slog.LevelError))
	})

	t.Run("rejects unknown levels", func(t *testing.T) {
		_, err := logging.New(t.Context(), &config.LogConfig{Level: "loud"})

		require.Error(t, err)
	})

	t.Run("fails when the file can't be opened", func(t *testing.T) {
		_, err := logging.New(t.Context(), &config.LogConfig{Path: t.TempDir(), Level: "info"})

		require.Error(t, err)
	})
}

func TestFromContext(t *testing.T) {
	t.Run("tags lines with the trace ID", func(t *testing.T) {
		var out bytes.Buffer
		ctx := logging.WithLogger(
			tracing.WithTraceID(t.Context(), "abc"),
			logging.NewWithWriter(t.Context(), &out, slog.LevelInfo),
		)

		logging.FromContext(ctx).InfoContext(ctx, "hello")

		require.Contains(t, out.String(), `"trace_id":"abc"`)
	})

	t.Run("falls back to a discarding logger", func(t *testing.T) {
		logger := logging.FromContext(t.Context())

		require.False(t, logger.Enabled(t.Context(), slog.LevelError))
	})
}
