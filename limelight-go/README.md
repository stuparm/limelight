# limelight-go

Go SDK. **The loop works end to end; the vet analyzer and the tag ladder do not exist yet.**

```
limelight.go, config.go, gate.go   registry, As(), the switch, the hot-path gate
emitter.go, logemitter.go          Emitter contract + the stdlib slog backend
control/                           http.Handler for the enable protocol
control/auto/                      optional pprof-style blank-import registration
analyzer/                          go/analysis pass — NOT IMPLEMENTED
internal/directive/                //limelight:method parser
internal/rewrite/                  the AST injection
internal/toolexec/                 the -toolexec driver
cmd/limelight/                     the shim binary
```

```
go get github.com/stuparm/limelight/limelight-go
```

## Using it

Register an extractor per field, install an emitter, mount the control endpoint:

```go
limelight.Register("projectID", project.IDFromContext, limelight.As("project.id"))
limelight.SetEmitter(limelight.NewLogEmitter())
control.Mount(mux)
```

`NewLogEmitter` takes a `*slog.Logger`, never a handler constructor, so every handler in
the ecosystem works without this package knowing about any of them:

```go
limelight.SetEmitter(limelight.NewLogEmitter(
	limelight.WithLogger(slog.New(tint.NewHandler(os.Stderr, nil))),
	limelight.WithLevel(slog.LevelDebug),
))
```

For a house logger, implement the interface — it has one method:

```go
limelight.SetEmitter(limelight.EmitterFunc(func(ctx context.Context, ev limelight.Event) {
	log.Info(ctx, "limelight", fieldsFrom(ev))
}))
```

Tag the methods. The directive names registered fields, never context keys:

```go
//limelight:method fields:"projectID,userID"
func (s *Service) CreateThing(ctx context.Context, req Request) error {
```

Build through the shim — this is what makes the tag do anything:

```
go build -o /tmp/limelight github.com/stuparm/limelight/limelight-go/cmd/limelight
go build -toolexec="/tmp/limelight toolexec" ./...
```

The shim rewrites in memory: nothing on disk changes, and a build without it produces a
binary that emits nothing.

## Two constraints worth knowing before you adopt it

**A package with a tagged method must already import this module.** `cmd/go` resolves
each package's dependencies before the shim runs and will not add to them, so the shim
fails with the import line to add rather than emitting code that cannot compile. A
package that only tags methods and never calls the runtime needs a blank import:

```go
import _ "github.com/stuparm/limelight/limelight-go"
```

That is one blank import per package, not per method — but it is a visible diff in domain
packages that otherwise have nothing to do with observability.

**This module is stdlib-only, and stays that way.** Because every tagged leaf package
imports it, a dependency here is a dependency everywhere. Backends that carry one
(`otel`, `zap`) get their own package and their own `go.mod`; they import this, never the
reverse.

If your router is not a `*http.ServeMux` — gin, echo, chi, gorilla — wrap it instead;
this works with anything that is an `http.Handler`:

```go
srv := &http.Server{Handler: control.Wrap(myRouter)}
```

`control.Handler()` returns the routes bare, for mounting them yourself behind your own
auth middleware or on an admin-only listener.

Or register it the way `net/http/pprof` does, as a side effect of an import:

```go
import _ "github.com/stuparm/limelight/limelight-go/control/auto"
```

That puts the routes on `http.DefaultServeMux`, so it only works if you serve that mux —
gin, echo, chi and grpc-gateway all serve their own, and against those the import compiles
and then 404s. It is a separate package, not an `init()` in `control/`, so that the import
site says what it is doing: importing `control/` arms nothing, importing `control/auto`
arms the endpoint.

It also leaves nowhere to put authentication, since the routes are registered before
`main` runs. The intended shape is a separate admin listener that is not routable from
outside — `examples/go` does exactly this, gin on `:8080` and limelight on `:6060`.

The endpoint is **not safe to expose as-is**: it turns on emission of user
identifiers and can flood a logging bill. Wrap it in authentication, a rate limit and an
audit log. It does enforce the two limits that hurt even an authenticated operator — a
1h TTL cap and a rejection of empty targeting.

## Costs, measured

go1.27.1, Apple M1 Pro:

| | |
|---|---|
| tagged call, switch off | ~1.2 ns, 0 allocations |
| inline budget | the gate takes a method from cost 5 to 69, against a budget of 80 |

A method whose own inline cost is ≤16 still inlines. That holds only while emission is
entry-only — entry + exit needs a `defer`, which is what makes the loss unconditional.
See [`../docs/design.md`](../docs/design.md).

Wire contract shared with every SDK: [`../spec/`](../spec/)
