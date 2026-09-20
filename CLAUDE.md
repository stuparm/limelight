# limelight

On-demand, identity-targeted method tracing: tag methods at compile time, flip a
runtime switch scoped to one identity and bounded by a TTL, and only those methods
emit product-semantic attributes into existing traces and logs.

**Status: the Go loop works end to end.** The `go vet` analyzer, the emitter
templates and the Java SDK are still stubs — do not assume a symbol exists
because a package is named after it.

## Where things live

- `spec/` — the wire contract shared by all SDKs. **Changes here are cross-language
  API changes**; they land before the SDK that consumes them, and every payload
  carries a `version` field that breaking changes bump.
- `docs/design.md` — rationale, prior art, and the costs we accepted. Read it before
  proposing an approach; the alternatives already rejected are listed there.
- `limelight-go/`, `limelight-java/` — per-language SDKs. Go is the only one needing
  a codemod (no annotations in the language).

## Conventions

- **A tag names a registered extractor, never a raw context key.** Idiomatic keys are
  unexported types in another package; generated code cannot reference them.
  See `spec/field-contract.md`.
- **Emitted attribute names follow OTel semantic conventions** — `user.id`, not
  `userID`. This is what makes the output joinable downstream.
- **The `Emitter` contract lives in the root package**, and a backend ships
  beside it only if it costs nothing but stdlib. Root is imported by every
  package holding a tagged method, so a dependency here is a dependency
  everywhere; otel/zap backends get their own package and `go.mod`.
- **A package with a tagged method must already import the runtime.** `cmd/go`
  resolves dependencies before `-toolexec` runs and will not add to them, so
  injected code can only call a package that is already in the build graph.
- Never put these high-cardinality fields on metric labels — span attributes and log
  fields only.

## Commands

The repo root is **not** a module, so `go build ./...` fails there. `go.work`
lists the four Go modules and wires the examples to the local SDK source.

```bash
go build ./limelight-go/... ./examples/go-log/... ./examples/go-log-zap/...   # from the root
cd limelight-go && go test ./...                # tests live here
cd limelight-java && mvn -q verify
```

A `//limelight:method` tag only does anything when the build goes through the
shim. A plain `go build` compiles it as a comment and emits nothing:

```bash
go build -o /tmp/limelight ./limelight-go/cmd/limelight
cd examples/go-log && go run -toolexec="/tmp/limelight toolexec" .
```

`LIMELIGHT_VERBOSE=1` makes the shim report each method it instruments.

## Decisions

A choice that constrains future work goes in `docs/design.md` — not in a code
comment, and not only in the commit message.
