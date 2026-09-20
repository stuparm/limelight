# Go example — zap

[`../go-log`](../go-log/) with one line changed:

```go
logger, _ := zap.NewProduction()
limelight.SetEmitter(limelightzap.New(logger))
```

Everything else is identical — the gin API on `:8080`, the identity middleware, the
tagged method, the admin listener on `:6060`. That is the point: the backend is the only
thing that varies.

## Run it

```
go build -o /tmp/limelight ../../limelight-go/cmd/limelight
go run -toolexec="/tmp/limelight toolexec" .
```

```
curl -sS -XPOST localhost:8080/api/things \
  -H 'X-Project-ID: abc-123' -H 'X-User-ID: u-42' -d '{"name":"widget"}'

curl -sS -XPOST localhost:6060/debug/limelight/enable \
  -d '{"version":0,"ttl":"30s","match":{"projectID":"abc-123"}}'
```

Repeat the first call and zap writes the event:

```json
{"level":"info","ts":1789830976.55,"caller":"limelightzap/limelightzap.go:83",
 "msg":"limelight","version":0,"method":"main.Service.CreateThing",
 "fields":{"project.id":"abc-123","user.id":"u-42"}}
```

Same schema as the slog example — `spec/event-schema.md` is what both emitters implement,
whatever their on-the-wire shape.

Note the `caller` field. `zap.NewProduction` adds it, and it points inside
`limelightzap` rather than at the tagged method, because that genuinely is the frame that
called zap. Read `method` instead: it carries `pkg.Receiver.Method`, derived at rewrite
time, which no stack frame can improve on. If you want `caller` to name the tagged method
anyway, pass a logger built with `zap.AddCallerSkip(2)`.

## Why `limelightzap` is a separate module

Every package containing a tagged method must import the limelight runtime, because
`-toolexec` cannot add to a package's build graph. So anything the runtime depends on
would land in the dependency graph of every instrumented package in a service. Keeping
zap in its own module means a service that logs with `slog` never sees it:

```
$ go list -m all        # in a service that uses limelight but not zap
theservice
github.com/stuparm/limelight/limelight-go v0.0.0
```

No `go.sum` at all — `limelight-go` has no dependencies.
