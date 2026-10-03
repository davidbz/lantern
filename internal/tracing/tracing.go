// Package tracing carries the trace ID that ties log lines of one operation together.
package tracing

import (
	"context"
	"crypto/rand"
)

type key int

const traceIDKey key = iota

func WithNewTraceID(ctx context.Context) context.Context {
	return WithTraceID(ctx, rand.Text())
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

func TraceID(ctx context.Context) string {
	traceID, _ := ctx.Value(traceIDKey).(string)
	return traceID
}
