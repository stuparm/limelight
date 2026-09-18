package limelight

import "context"

// EventVersion is the event schema version. Breaking changes bump it; see
// spec/event-schema.md.
//
// It is not the SDK's version, and it is not control.ProtocolVersion — the wire
// format of the enable endpoint and the shape of an emitted event version
// independently.
const EventVersion = 0

// Event is the shape every emitter must produce, whatever its backend's
// on-the-wire format. See spec/event-schema.md.
type Event struct {
	Version int    `json:"version"`
	Method  string `json:"method"`
	// TraceID and SpanID are what make the output joinable to existing traces.
	// NewLogEmitter leaves them empty: reading them needs an OpenTelemetry
	// dependency, and this package stays stdlib-only. A backend that already
	// has the span — or a host logger that correlates traces itself — fills
	// them in.
	TraceID string            `json:"trace_id,omitempty"`
	SpanID  string            `json:"span_id,omitempty"`
	Fields  map[string]string `json:"fields"`
}

// Emitter is the one thing a backend has to implement.
//
// It is declared here, in the package that consumes it, rather than beside an
// implementation. That is not only Go's usual habit: every package containing a
// tagged method must import this one, so anything this package depends on lands
// in the dependency graph of every instrumented leaf package. A backend
// carrying a third-party dependency therefore lives in its own package —
// limelightotel, limelightzap — which imports this one and is never imported
// back.
type Emitter interface {
	Emit(ctx context.Context, ev Event)
}

// EmitterFunc adapts a plain function to Emitter. It is the escape hatch for a
// house logger:
//
//	limelight.SetEmitter(limelight.EmitterFunc(func(ctx context.Context, ev limelight.Event) {
//		log.Info(ctx, "limelight", fieldsFrom(ev))
//	}))
type EmitterFunc func(ctx context.Context, ev Event)

// Emit implements Emitter.
func (f EmitterFunc) Emit(ctx context.Context, ev Event) { f(ctx, ev) }

// DiscardEmitter drops every event. It is what SetEmitter installs for a nil
// emitter, and what an instrumented binary that never configured one uses — so
// the default is silence rather than output in a place nobody chose.
var DiscardEmitter Emitter = EmitterFunc(func(context.Context, Event) {})
