# Examples

Runnable services that import the SDKs the way a real codebase would — one per language,
each its own module so its dependencies never leak back into the SDK.

| | |
|---|---|
| [`go/`](go/) | imports `limelight-go`; also the fixture the codemod and the vet analyzer are developed against |

A `go.work` at the repo root wires `examples/go` to the local `limelight-go` source, so
edits to the SDK are picked up with no publish step and no `replace` directive.
