# limelight-go

Go SDK. **Structure only — the packages compile, nothing is implemented.**

```
limelight.go, config.go, registry.go   registry, As(), the switch, the hot-path gate
control/                               http.Handler for POST /debug/limelight/enable
emit/                                  emitter interface + generated backends
analyzer/                              go/analysis pass, exported for your vet binary
internal/directive/                    //limelight:method parser
internal/rewrite/                      AST codemod
templates/                             slog.go.tmpl, otel.go.tmpl, ...
cmd/limelight/                         limelight -w ./...
```

Go is the only target language that needs a codemod — it has no annotations, so a tag is
either a comment (invisible to the compiler) or real code, with nothing in between.

```
go get github.com/stuparm/limelight/limelight-go
```

A runnable service that imports this module lives in [`../examples/go`](../examples/go); `go.work` at the repo root points it at this source.

Wire contract shared with every SDK: [`../spec/`](../spec/)

## Costs, stated up front

- **Lost inlining**, permanently, on or off — the injected `defer` does it.
- ~1–2 ns per tagged call when off: one atomic load and a branch.
- A REST toggle behind a load balancer enables **one pod**. See
  [`../spec/enable-protocol.md`](../spec/enable-protocol.md).
