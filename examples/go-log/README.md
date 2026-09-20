# Go example — slog

A service shaped like a real one: a **gin** REST API on `:8080`, and limelight's control
endpoint on a **separate admin listener** on `:6060`. Two listeners, two ports, one
process — they cannot collide.

That split is the point. The control endpoint turns on emission of user identifiers, so
production binds it to loopback or an internal interface and never routes it from
outside. It is the same shape `net/http/pprof` recommends, and for the same reason.

## Run it

The directive only does something when the build goes through the shim:

```
go build -o /tmp/limelight ../../limelight-go/cmd/limelight
go run -toolexec="/tmp/limelight toolexec" .
```

Create a thing — this is the app's own API, nothing to do with limelight:

```
curl -sS -XPOST localhost:8080/api/things \
  -H 'X-Project-ID: abc-123' -H 'X-User-ID: u-42' -d '{"name":"widget"}'
```

Now trace one project for thirty seconds, on the admin port:

```
curl -sS -XPOST localhost:6060/debug/limelight/enable \
  -d '{"version":0,"ttl":"30s","match":{"projectID":"abc-123"}}'
```

Repeat the two `POST /api/things` calls, one as `abc-123` and one as any other project:

```
{"time":"…","level":"INFO","msg":"limelight","version":0,
 "method":"main.Service.CreateThing","fields":{"project.id":"abc-123","user.id":"u-42"}}
```

Only the targeted project emits. Every other request runs through the same tagged method
and stays silent. Thirty seconds later the switch turns itself off.

If you do not know an identity yet — an incident, or just checking the instrumentation
works at all — ask for everything instead:

```
curl -sS -XPOST localhost:6060/debug/limelight/enable \
  -d '{"version":0,"ttl":"10s","match_all":true}'
```

`match_all` is a named field rather than an empty `match`, because an empty map is what a
dropped field marshals to and the cheapest bug should not produce the most expensive
outcome. It is capped at 60s against the hour allowed for a targeted trace.

`GET /debug/limelight/status` reports whether it is on and until when;
`POST /debug/limelight/disable` stops it early. Both on `:6060`.

## How the two servers are wired

The app builds its own gin engine and serves it on `:8080`. limelight is registered by a
blank import:

```go
import _ "github.com/stuparm/limelight/limelight-go/control/auto"
```

which puts `/debug/limelight/` on `http.DefaultServeMux`, and the admin goroutine serves
that mux on `:6060`. The gin engine never sees those routes, and `DefaultServeMux` never
sees the app's.

If you would rather not use a second listener, the same routes mount onto a router you
already have — `control.Mount(mux)` for a `*http.ServeMux`, `control.Wrap(router)` for
gin/echo/chi, or `control.Handler()` to put your own auth middleware in front.

## The identity middleware

`identity()` lifts `X-Project-ID` and `X-User-ID` off the request into the context; the
extractors registered in `main` read them back out. Every service already has a middleware
like this — which is the whole reason a tag names a **registered extractor** rather than a
context key: `projectIDKey{}` is unexported and generated code could never reference it.

## Run it without the shim

```
go run .
```

The API behaves identically and emits nothing: `//limelight:method` is an ordinary
comment and the compiler ignores it. That silence is the failure mode the vet analyzer
exists to catch — and the analyzer is not built yet, so for now the only signal is
`LIMELIGHT_VERBOSE=1`, which makes the shim report each method it instruments.
