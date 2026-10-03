package tracing_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/tracing"
)

func TestWithNewTraceID(t *testing.T) {
	first := tracing.TraceID(tracing.WithNewTraceID(t.Context()))
	second := tracing.TraceID(tracing.WithNewTraceID(t.Context()))

	require.NotEmpty(t, first)
	require.NotEqual(t, first, second)
	require.Empty(t, tracing.TraceID(t.Context()))
}
