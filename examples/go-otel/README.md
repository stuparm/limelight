# Go example — OpenTelemetry

[`../go-log`](../go-log/) with a tracer added, and **the base sampler set to
`NeverSample` on purpose**. Nothing in this service is traced unless limelight asks for
it, which makes the one thing the otel integration is for visible in a terminal.

Spans print to stdout through `stdouttrace` — no collector, no docker.

## Run it

```
go build -o /tmp/limelight ../../limelight-go/cmd/limelight
go run -toolexec="/tmp/limelight toolexec" .
```

**1. A request while the switch is off.** A log line, and no span:

```
curl -sS -XPOST localhost:8080/api/things \
  -H 'X-Project-ID: abc-123' -H 'X-User-ID: u-42' -d '{"name":"widget"}'
```

**2. Turn it on for one project:**

```
curl -sS -XPOST localhost:6060/debug/limelight/enable \
  -d '{"version":0,"ttl":"30s","match":{"projectID":"abc-123"}}'
```

**3. Repeat step 1.** Now the log line carries a `trace_id`, and a span is printed:

```
Name: Service.CreateThing
Attributes: {project.id: abc-123, user.id: u-42}
Events: [limelight {limelight.method: main.Service.CreateThing}]
```

**4. The same request as another project** stays untraced — no span, no event.

## Why the HTTP span is missing

The trace you get is partial: `Service.CreateThing` is there, the `POST /api/things`
span above it is not. That is not a bug, it is the ordering every real service has, and
`main.go` reproduces it deliberately:

```go
r.Use(gin.Recovery(), tracing(tracer), identity())
//                    ^^^^^^^ runs first
```

A sampling decision is made when a span is created. The server span is created before
the identity middleware runs, so at that moment nobody knows whose request it is,
limelight cannot ask for it, and the base sampler drops it. Every span created *after*
identity is known overrides that inherited "no" — which is what
`limelightotel.Sampler` does, and why the tagged work survives while its parent does not.

A service that can extract identity before its tracing middleware gets whole traces
instead, with no change to the sampler.

## The test

`main_test.go` asserts the chain end to end: the tag fires, the event carries a trace id,
the identity lands on the span as attributes, and an untargeted request produces nothing.

It only means anything when built through the shim:

```
go test -toolexec="/tmp/limelight toolexec" ./...
```

A plain `go test` compiles `//limelight:method` as an ordinary comment, so the tag never
fires. The test detects that and fails with these instructions rather than passing while
proving nothing.
