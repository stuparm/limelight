package limelight

import (
	"context"
	"log/slog"
	"sort"
)

// logEmitter writes events to a *slog.Logger.
//
// It lives in this package because it costs nothing but stdlib, which is the
// rule for what may sit next to the Emitter contract. A backend that would add
// a dependency belongs in its own package.
type logEmitter struct {
	logger *slog.Logger
	level  slog.Leveler
}

// LogOption configures the emitter returned by NewLogEmitter.
type LogOption func(*logEmitter)

// WithLogger sets the logger to write to. Without it, events go to
// slog.Default() as it stands at emit time, so a process that calls
// slog.SetDefault after start-up is still honoured.
func WithLogger(l *slog.Logger) LogOption {
	return func(e *logEmitter) { e.logger = l }
}

// WithLevel sets the level events are logged at. The default is slog.LevelInfo.
func WithLevel(l slog.Leveler) LogOption {
	return func(e *logEmitter) { e.level = l }
}

// NewLogEmitter returns an Emitter backed by log/slog.
//
//	limelight.SetEmitter(limelight.NewLogEmitter())
//
//	limelight.SetEmitter(limelight.NewLogEmitter(
//		limelight.WithLogger(slog.New(tint.NewHandler(os.Stderr, nil))),
//		limelight.WithLevel(slog.LevelDebug),
//	))
//
// It takes a *slog.Logger rather than wrapping slog's handler constructors, so
// every handler in the ecosystem works here without this package knowing about
// any of them.
func NewLogEmitter(opts ...LogOption) Emitter {
	e := &logEmitter{level: slog.LevelInfo}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Emit implements Emitter. Fields are emitted as a group under "fields" and
// sorted by key, so two runs of the same code produce identical lines.
func (e *logEmitter) Emit(ctx context.Context, ev Event) {
	logger := e.logger
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

	logger.LogAttrs(ctx, e.level.Level(), "limelight", attrs...)
}
