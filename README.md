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

Design settled, nothing implemented. The directory skeleton compiles; that is all.

## License

Apache-2.0
