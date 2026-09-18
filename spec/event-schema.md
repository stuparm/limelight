# Event schema (v0 draft)

What a tagged method emits while the switch is on for the matching identity.

Always include `trace_id` and `span_id` when a span is in flight — the join to existing
traces depends on them, and without it the output is just more logs.

```json
{
  "version": 0,
  "method": "vnet.Service.CreateThing",
  "trace_id": "...",
  "span_id": "...",
  "fields": { "user.id": "u-42", "project.id": "abc-123" }
}
```

Emitters are templates, not hardcoded OTel: ship `slog`, `otel` and `zap`
implementations and let teams write one for their house logger. The schema above is
what every template must produce, whatever the backend's on-the-wire shape.

## Settled in v0 by the Go SDK

- **Entry-only.** Entry + exit would carry duration and error, but it needs a `defer` in
  every tagged method, and the `defer` is what turns the inlining cost from a budget
  charge into an unconditional loss. See [`../docs/design.md`](../docs/design.md). Not
  closed — reopen it with a measurement, not an argument.
- **The method name is derived, not declared:** `<package>.<Receiver>.<Method>`, or
  `<package>.<Function>` for a plain function. The directive names fields and nothing
  else, so two SDKs cannot disagree about what a method is called.

## Still open

- `trace_id` / `span_id` are specified here but the slog emitter leaves them empty:
  reading them needs an OpenTelemetry dependency and the runtime package is
  dependency-light. The otel emitter is where they get filled in, and until it exists the
  join this schema promises is not real.
