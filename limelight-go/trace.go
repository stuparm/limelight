package limelight

import (
	"context"
	"sync/atomic"
)

// TraceContextFunc pulls the active trace and span ids out of a context. Both
// results are empty when no span is in flight.
type TraceContextFunc func(ctx context.Context) (traceID, spanID string)

type traceContextHolder struct{ f TraceContextFunc }

var traceContext atomic.Pointer[traceContextHolder]

// SetTraceContextFunc installs the function that fills Event.TraceID and
// Event.SpanID.
//
// It exists because this package is stdlib-only and cannot read an
// OpenTelemetry span itself, while the join to existing traces — the thing
// spec/event-schema.md is built around — needs exactly that. limelightotel
// registers an implementation; once it has, *every* emitter gets trace ids,
// including the slog and zap backends and any hand-written EmitterFunc.
//
// A nil f removes the hook.
func SetTraceContextFunc(f TraceContextFunc) {
	if f == nil {
		traceContext.Store(nil)
		return
	}
	traceContext.Store(&traceContextHolder{f: f})
}
