# limelight

**On-demand, identity-targeted method tracing.**

> *"Trace everything for customer X for the next 10 minutes."*

Tag methods once, at compile time. Then flip a runtime switch — scoped to one identity,
bounded by a TTL — and only those methods start emitting product-semantic attributes
(`user.id`, `project.id`, `http.request.id`) into the traces and logs you already have.
When the TTL expires it turns itself off.

```go
//limelight:method fields:"requestID,projectID"
func (s *Service) CreateThing(ctx context.Context, req Request) error {
```

```http
POST /debug/limelight/enable
{ "version": 0, "ttl": "10m", "match": { "projectID": "abc-123" } }
```

The tag names a **registered extractor**, not a raw context key — which is what makes it
work in any codebase:

```go
limelight.Register("projectID", project.IDFromContext, limelight.As("project.id"))
```

Compile-time method tagging is table stakes; otelc, orchestrion and go-instrument all do
it. **The targeted, expiring switch is the point.**

## Try it

The tag takes effect through `-toolexec`, so an ordinary build applies it and no rewritten
source is ever left on disk.

```bash
git clone https://github.com/stuparm/limelight
cd limelight/examples/go-log

go build -o /tmp/limelight ../../limelight-go/cmd/limelight
go run -toolexec="/tmp/limelight toolexec" .
```

That starts a small REST API on `:8080` and limelight's control endpoint on `:6060`.
Create something, as one project:

```bash
curl -sS -XPOST localhost:8080/api/things \
  -H 'X-Project-ID: abc-123' -H 'X-User-ID: u-42' -d '{"name":"widget"}'
```

Nothing is emitted. Now turn it on for that one project, for thirty seconds:

```bash
curl -sS -XPOST localhost:6060/debug/limelight/enable \
  -d '{"version":0,"ttl":"30s","match":{"projectID":"abc-123"}}'
```

Repeat the first call and the event appears. Repeat it as any other project and nothing
does. Thirty seconds later it stops on its own.

Build without the shim — a plain `go run .` — and the directive is an ordinary comment:
the service behaves identically and emits nothing.

To use it in your own service, install the shim and wire it into your build:

```bash
go install github.com/stuparm/limelight/limelight-go/cmd/limelight@latest
go build -toolexec="$(go env GOPATH)/bin/limelight toolexec" ./...
```

## Layout

| | |
|---|---|
| [`spec/`](spec/) | the wire contract every SDK shares — field contract, enable protocol, event schema |
| [`limelight-go/`](limelight-go/) | Go SDK: registry, switch, `-toolexec` shim, slog emitter. Stdlib-only, by construction — its `go.mod` has no `require` block |
| [`limelight-go/limelightzap/`](limelight-go/limelightzap/) | zap backend, its own module so the dependency reaches only services that want it |
| [`limelight-java/`](limelight-java/) | **planned.** A `pom.xml` and a package doc comment — no Java yet |
| [`examples/`](examples/) | runnable services — [`go-log`](examples/go-log/) and [`go-log-zap`](examples/go-log-zap/), differing only in the emitter |
| [`docs/design.md`](docs/design.md) | why it is built this way, and the prior art |

The spec is written as its own thing, ahead of a second SDK, because context models
diverge per language while the field contract and the enable protocol do not. Today only
the Go SDK exists.

## Status

**The Go loop works end to end**, and the section above is that walkthrough: tag a
method, build through the shim, POST the enable endpoint, watch one identity emit while
every other request through the same method stays silent, watch it stop at the TTL.

Working: the registry and `As` mapping, the TTL-bounded switch, the `-toolexec` shim, the
enable/disable/status endpoint, and emitters for `log/slog` and zap.

Not built yet. The first two are why this is not v1 — both let a mistake pass in silence,
which is the one thing a tool like this cannot afford:

- the **`go vet` analyzer**. A directive is a comment, so nothing in the Go toolchain
  objects to a tag that cannot work — a method with no `context.Context`, or a `fields:`
  name nobody registered. Without the analyzer you find out in production, when you flip
  the switch during an incident and nothing comes out.
- the **tag ladder**. Only `//limelight:method` exists. Write `//limelight:type` and it is
  parsed as an ordinary comment and discarded without a word — a worse version of the same
  problem, since it looks like a supported feature.
- **`trace_id` / `span_id`** are always empty. Reading them needs an OpenTelemetry
  dependency the runtime does not carry, so the join to existing traces that
  [`spec/event-schema.md`](spec/event-schema.md) describes is not real yet.
- the **`methods` glob** in the enable protocol returns 501.
- **limelight-java** is a skeleton.

## License

Apache-2.0
