# Examples

Runnable services that import the SDKs the way a real codebase would. Each is its own
module, so an example's dependencies never leak back into the SDK — that is the whole
reason they are not subpackages.

| | |
|---|---|
| [`go-log/`](go-log/) | gin REST API, events emitted through `log/slog`. Start here. |
| [`go-log-zap/`](go-log-zap/) | the same service with one line changed: the emitter is `limelightzap` |
| [`go-otel/`](go-otel/) | events joined into an OpenTelemetry pipeline — trace ids on the log line, identity as span attributes, and force-sampling so a trace exists to write to |

The pair exists to show that the backend is the only thing that varies. The tag, the
registry, the switch and the control endpoint are identical in both — swapping where
events go does not touch how they are produced.

`go-otel` also carries `main_test.go`, an integration test of the whole chain — tag,
emitter, span. It must be run as `go test -toolexec="/tmp/limelight toolexec" ./...`; a
plain `go test` builds an uninstrumented binary, and the test fails loudly rather than
passing on nothing.

`go-log` is also the fixture the `-toolexec` shim is developed against: it carries a real
`//limelight:method` directive, so the rewriter has something to chew on from the first
commit.

A `go.work` at the repo root wires both to the local `limelight-go` source, so edits to
the SDK are picked up with no publish step.
