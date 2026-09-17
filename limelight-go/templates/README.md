# Emitter templates

One template per backend — `slog.go.tmpl` first, then `otel.go.tmpl`, `zap.go.tmpl`.
Teams write their own for their house logger. Every template must produce the shape in
[`../../spec/event-schema.md`](../../spec/event-schema.md), including trace/span ids.
