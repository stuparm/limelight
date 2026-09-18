# limelight

**On-demand, identity-targeted method tracing.**

> *"Trace everything for customer X for the next 10 minutes."*

Tag methods once, at compile time. Then flip a runtime switch — scoped to one identity,
bounded by a TTL — and only those methods start emitting product-semantic attributes
(`user.id`, `project.id`, request id) into the traces and logs you already have.
When the TTL expires it turns itself off.

```go
//limelight:method fields:"requestID,projectID"
func (s *Service) CreateThing(ctx context.Context, req Request) error {
```

```http
POST /debug/limelight/enable
{ "version": 0, "ttl": "10m", "match": { "projectID": "abc-123" } }
```

The tag takes effect through `-toolexec`, so an ordinary build applies it and no
rewritten source is ever left on disk:

```
go run -toolexec="/tmp/limelight toolexec" .
```

The tag names a **registered extractor**, not a raw context key — which is what makes it
work in any codebase:

```go
limelight.Register("projectID", project.IDFromContext, limelight.As("project.id"))
```

Compile-time method tagging is table stakes; otelc, orchestrion and go-instrument all do
it. **The targeted, expiring switch is the point.**

## Layout

| | |
|---|---|
| [`spec/`](spec/) | the wire contract every SDK shares — field contract, enable protocol, event schema |
| [`limelight-go/`](limelight-go/) | Go SDK: registry, switch, codemod, vet analyzer |
| [`limelight-java/`](limelight-java/) | Java SDK: registry, switch, `@Limelight` |
| [`examples/`](examples/) | runnable services that import the SDKs — `examples/go` is wired to the local source by `go.work` |
| [`docs/design.md`](docs/design.md) | why it is built this way, and the prior art |

The spec is the reason both SDKs live in one repo: context models diverge per language,
the field contract and the enable protocol do not.

## Status

**The Go loop works end to end**: tag a method, build through the shim, POST the enable
endpoint, watch the targeted identity emit and nothing else, watch it stop at the TTL.
[`examples/go`](examples/go/) is that walkthrough.

Not built yet, and each one is load-bearing for a real adoption:

- the **`go vet` analyzer** — without it a tag that cannot work is silently ignored,
  which is the failure mode limelight exists to prevent
- the **tag ladder** — only `//limelight:method` exists; `:package`, `:type` and `:skip`
  are parsed as non-directives and ignored **with no warning at all**, which is the exact
  failure mode limelight exists to prevent
- **`trace_id` / `span_id`** — the slog emitter leaves them empty, so the join to
  existing traces that [`spec/event-schema.md`](spec/event-schema.md) promises is not
  real yet
- the **`methods` glob** in the enable protocol, which returns 501
- **limelight-java** — still a skeleton

## License

Apache-2.0
