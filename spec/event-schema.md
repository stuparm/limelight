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

## Open in v0

- Entry-only, or entry + exit with duration and error?
- Does the method name follow a convention, or is it whatever the directive says?
