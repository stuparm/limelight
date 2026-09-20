// Package limelightzap emits limelight events through a zap logger.
//
//	logger, _ := zap.NewProduction()
//	limelight.SetEmitter(limelightzap.New(logger))
//
// It is a separate module, not a package inside limelight-go, and that is the
// point. Every package containing a tagged method must import the limelight
// runtime — a consequence of instrumenting through -toolexec, which cannot add
// to a package's build graph. So anything the runtime depends on lands in the
// dependency graph of every instrumented leaf package in a service. Keeping zap
// out here means a service that logs with slog never sees zap in its go.sum,
// and vice versa.
//
// This module imports limelight-go. limelight-go never imports this.
package limelightzap

import (
	"context"
	"sort"

	limelight "github.com/stuparm/limelight/limelight-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type emitter struct {
	logger *zap.Logger
	level  zapcore.Level
}

// Option configures the emitter returned by New.
type Option func(*emitter)

// WithLevel sets the level events are logged at. The default is
// zapcore.InfoLevel.
func WithLevel(level zapcore.Level) Option {
	return func(e *emitter) { e.level = level }
}

// New returns an Emitter that writes events to l.
//
// The logger is positional here, where limelight.NewLogEmitter takes it as an
// option, because the two globals behave differently: slog.Default() writes to
// stderr, so falling back to it is useful, whereas zap.L() is a no-op logger
// until something calls zap.ReplaceGlobals. A nil l still falls back to zap.L()
// at emit time — so that a process which does call ReplaceGlobals is honoured —
// but passing nil without doing so discards every event silently.
func New(l *zap.Logger, opts ...Option) limelight.Emitter {
	e := &emitter{logger: l, level: zapcore.InfoLevel}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Emit implements limelight.Emitter.
//
// The context is unused: zap has no context-aware logging call. A host logger
// that correlates traces off the context — as several house loggers do — is a
// reason to write an EmitterFunc instead of using this.
//
// On a logger built with zap.AddCaller (which zap.NewProduction sets), the
// caller field on these lines points here rather than at the tagged method.
// That is left alone deliberately: Event.Method carries pkg.Receiver.Method,
// derived at rewrite time, which is more accurate than any stack frame. A
// caller frame would only ever name the injected call site. If you would rather
// it pointed at the tagged method anyway, hand New a logger built with
// zap.AddCallerSkip(2) — one frame for limelight.Emit, one for this method.
func (e *emitter) Emit(_ context.Context, ev limelight.Event) {
	logger := e.logger
	if logger == nil {
		logger = zap.L()
	}
	if !logger.Core().Enabled(e.level) {
		return
	}

	fields := make([]zap.Field, 0, 5)
	fields = append(fields,
		zap.Int("version", ev.Version),
		zap.String("method", ev.Method),
	)
	if ev.TraceID != "" {
		fields = append(fields, zap.String("trace_id", ev.TraceID))
	}
	if ev.SpanID != "" {
		fields = append(fields, zap.String("span_id", ev.SpanID))
	}
	fields = append(fields, zap.Object("fields", eventFields(ev.Fields)))

	logger.Log(e.level, "limelight", fields...)
}

// eventFields writes the extracted fields as a nested object under "fields",
// matching spec/event-schema.md. Keys are sorted so two runs of the same code
// produce identical lines, the same guarantee limelight.NewLogEmitter gives.
type eventFields map[string]string

func (f eventFields) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		enc.AddString(k, f[k])
	}
	return nil
}
