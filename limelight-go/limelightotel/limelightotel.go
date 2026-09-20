// Package limelightotel puts limelight's output into an OpenTelemetry pipeline
// that a service already runs.
//
// It does three separable things:
//
//   - InstallTraceContext makes every emitter — slog, zap, a hand-written
//     EmitterFunc — carry trace_id and span_id, so log lines join to traces.
//   - New returns an Emitter that writes the extracted fields onto the live
//     span as attributes, and records one span event per tagged method.
//   - Sampler force-samples requests limelight is targeting, so a trace exists
//     to write to in the first place.
//
// Each works without the others. A service that only wants the log-side join
// installs the hook and nothing else.
package limelightotel

import (
	"context"
	"sort"

	limelight "github.com/stuparm/limelight/limelight-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// EventName is the span event recorded for each tagged method.
const EventName = "limelight"

// MethodAttr carries the tagged method's name on that span event.
const MethodAttr = "limelight.method"

// TraceContext reports the trace and span ids on ctx, or two empty strings when
// no valid span is in flight.
func TraceContext(ctx context.Context) (traceID, spanID string) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return "", ""
	}
	return sc.TraceID().String(), sc.SpanID().String()
}

// InstallTraceContext wires TraceContext into the limelight runtime, so every
// emitted event carries trace_id and span_id.
//
// This is the cheapest half of the integration and the one that survives
// sampling: a log line is written whether or not the trace was sampled, whereas
// anything written onto a span is lost when it is not recording.
func InstallTraceContext() {
	limelight.SetTraceContextFunc(TraceContext)
}

type emitter struct{}

// New returns an Emitter that writes onto the live span.
//
// The extracted fields become span attributes — they are request-scoped facts
// with the same value every time, so writing them repeatedly is idempotent and
// they stay queryable. The per-method record becomes a span event, which is
// ordered and timestamped and does not collide when several tagged methods run
// inside one span.
//
// It emits nothing when no span is recording. Pair it with a logging emitter
// through limelight.MultiEmitter if you want output either way.
func New() limelight.Emitter { return emitter{} }

// Emit implements limelight.Emitter.
func (emitter) Emit(ctx context.Context, ev limelight.Event) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	attrs := fieldAttrs(ev.Fields)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	span.AddEvent(EventName, trace.WithAttributes(attribute.String(MethodAttr, ev.Method)))
}

// fieldAttrs converts the extracted fields to attributes, sorted by key so two
// runs of the same code produce the same ordering.
func fieldAttrs(fields map[string]string) []attribute.KeyValue {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	attrs := make([]attribute.KeyValue, 0, len(keys))
	for _, k := range keys {
		attrs = append(attrs, attribute.String(k, fields[k]))
	}
	return attrs
}
