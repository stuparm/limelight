# limelight-go

Go SDK. **The loop works end to end; the vet analyzer and the emitter templates do not
exist yet.**

```
registry.go, config.go, gate.go   registry, As(), the switch, the hot-path gate
control/                          http.Handler for the enable protocol
emit/                             Emitter interface + slog backend
analyzer/                         go/analysis pass — NOT IMPLEMENTED
internal/directive/               //limelight:method parser
internal/rewrite/                 the AST injection
internal/toolexec/                the -toolexec driver
cmd/limelight/                    the shim binary
templates/                        emitter templates — NOT IMPLEMENTED
```

```
go get github.com/stuparm/limelight/limelight-go
```

## Using it

Register an extractor per field, install an emitter, mount the control endpoint:

```go
limelight.Register("projectID", project.IDFromContext, limelight.As("project.id"))
limelight.SetEmitter(emit.Slog(slog.Default()))
mux.Handle(control.Prefix, control.Handler())
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

**A package with a tagged method must already import this module.** `cmd/go` resolves
each package's dependencies before the shim runs and will not add to them, so the shim
fails with the import line to add rather than emitting code that cannot compile. A
package that only tags methods and never calls the runtime needs a blank import:

```go
import _ "github.com/stuparm/limelight/limelight-go"
```

`control.Handler()` is **not safe to expose as-is**: it turns on emission of user
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
