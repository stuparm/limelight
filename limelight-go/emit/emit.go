// Package emit turns extracted fields into output. The emitter is an interface
// so the backend is a choice — slog first, then otel, zap, or whatever a team
// writes for its house logger.
//
// Keeping this behind an interface is what leaves the baggage-vs-custom-emitter
// question (docs/design.md) open until there is evidence.
package emit

import (
	"context"
	"log/slog"
	"sort"
)

// Version is the event schema version. Breaking changes bump it; see
// spec/event-schema.md.
const Version = 0

// Event is the shape every emitter must produce, whatever its backend's
// on-the-wire format. See spec/event-schema.md.
type Event struct {
	Version int    `json:"version"`
	Method  string `json:"method"`
	// TraceID and SpanID are what make the output joinable to existing traces.
	// The slog emitter leaves them empty: reading them needs an OpenTelemetry
	// dependency, and the runtime package stays dependency-light. The otel
	// emitter fills them in.
	TraceID string            `json:"trace_id,omitempty"`
	SpanID  string            `json:"span_id,omitempty"`
	Fields  map[string]string `json:"fields"`
}

// Emitter is the one thing a backend has to implement.
type Emitter interface {
	Emit(ctx context.Context, ev Event)
}

// Func adapts a plain function to Emitter.
type Func func(ctx context.Context, ev Event)

// Emit implements Emitter.
func (f Func) Emit(ctx context.Context, ev Event) { f(ctx, ev) }

// Slog returns an Emitter that writes events to l. A nil l means
// slog.Default() at emit time.
//
// Fields are emitted as a group under "fields" and sorted by key, so two runs
// of the same code produce byte-identical lines.
func Slog(l *slog.Logger) Emitter {
	return Func(func(ctx context.Context, ev Event) {
		logger := l
		if logger == nil {
			logger = slog.Default()
		}
		attrs := make([]slog.Attr, 0, 5)
		attrs = append(attrs,
			slog.Int("version", ev.Version),
			slog.String("method", ev.Method),
		)
		if ev.TraceID != "" {
			attrs = append(attrs, slog.String("trace_id", ev.TraceID))
		}
		if ev.SpanID != "" {
			attrs = append(attrs, slog.String("span_id", ev.SpanID))
		}

		keys := make([]string, 0, len(ev.Fields))
		for k := range ev.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		group := make([]slog.Attr, 0, len(keys))
		for _, k := range keys {
			group = append(group, slog.String(k, ev.Fields[k]))
		}
		attrs = append(attrs, slog.Attr{Key: "fields", Value: slog.GroupValue(group...)})

		logger.LogAttrs(ctx, slog.LevelInfo, "limelight", attrs...)
	})
}

// Discard is an Emitter that drops everything. It is the default until a
// process calls limelight.SetEmitter, so a binary that was instrumented but
// never configured stays silent instead of writing to an unexpected place.
var Discard Emitter = Func(func(context.Context, Event) {})
