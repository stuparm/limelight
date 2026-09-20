# limelight

A **Go** SDK for on-demand, identity-targeted method tracing: tag methods at compile
time, flip a TTL-bounded runtime switch scoped to one identity, and only those methods
emit product-semantic attributes into the traces and logs a service already has.

`spec/` is written to be language-neutral because **Java is intended to follow** — but it
does not exist yet. `limelight-java/` is a `pom.xml` and a package doc comment with no
Java in it: no `@Limelight`, no registry, no switch. Treat every claim about it as a plan.

**Status: the Go loop works end to end.** The `go vet` analyzer and the tag ladder
(`:package`/`:type`/`:skip`) are also unbuilt — do not assume a symbol exists because a
package is named after it.

## Rules

**1. Verify by running, never by reading.** This repo's whole failure mode is code that
compiles and silently does nothing. A change is done when a command was executed and its
output examined — not when the diff looks right. Paste the output.

**2. A change to the rewriter, the gate, the directive parser or the wire protocol lands
with a test that fails without it.** These four are where a silent regression is
invisible.

**3. A test that can pass against an uninstrumented binary is not a test.** `go test`
without `-toolexec` compiles `//limelight:method` as a comment. Any test asserting the
tag's behaviour must detect that and fail loudly — see `examples/go-otel/main_test.go`.

**4. `limelight-go/go.mod` has no `require` block, and never will.** Every package
holding a tagged method must import the runtime, so a dependency there is a dependency in
every instrumented leaf package of every adopting service. Check it after touching root.

**5. A backend carrying a third-party dependency gets its own module** — `limelightzap`,
`limelightotel`. They import root; root never imports them. An emitter ships beside the
`Emitter` contract only if it costs nothing but stdlib.

**6. A tag names a registered extractor, never a raw context key.** Idiomatic keys are
unexported types in another package and generated code cannot reference them. See
`spec/field-contract.md`.

**7. Emitted attribute names follow OTel semantic conventions** — `user.id`, not
`userID`. The downstream join is the entire product; without the mapping it does not
happen. Never put these high-cardinality fields on metric labels.

**8. Protocol JSON is snake_case** — `match_all`, `expires_at`, `trace_id`. The keys
*inside* `match` are the field names a service registered and look however that codebase
spells them. Do not normalise those.

**9. Nothing may fail silently.** A directive that cannot work must produce a build error
or a warning — never nothing. This is the one rule the project exists to enforce, and the
one it currently breaks: `//limelight:type` is discarded without a word.

**10. The off-path budget is ~1.2 ns and zero allocations.** Benchmark before and after
touching `Emit`, `On` or the injected call shape. The guard is why generated code is
`if limelight.On() { limelight.Emit(...) }` and not a bare call.

**11. Emission is entry-only, and there is no `defer`.** That is what keeps the inlining
cost a budget charge (5 → 69 against 80) rather than an unconditional loss. Adding
entry+exit reopens that trade — discuss before building it.

**12. A choice that constrains future work goes in `docs/design.md`,** not in a code
comment and not only in a commit message. A `spec/` change is a cross-language API change
and bumps `version`.

**13. Never `git commit` or `git push` unsolicited.** Stage and describe changes when
asked.

## Routing

| Looking for | Go to |
|---|---|
| why it is built this way, rejected alternatives, measured costs | `docs/design.md` |
| the wire contract every SDK shares | `spec/` — field contract, enable protocol, event schema |
| registry, switch, gate, `Emitter`, slog backend | `limelight-go/` root package |
| the `-toolexec` shim and AST injection | `limelight-go/internal/{toolexec,rewrite,directive}` |
| the enable/disable/status endpoint | `limelight-go/control/`, `control/auto/` for blank-import registration |
| zap and OpenTelemetry backends | `limelight-go/limelightzap/`, `limelight-go/limelightotel/` |
| a runnable service, and the shim's test fixture | `examples/go-log`, `go-log-zap`, `go-otel` |
| what is deliberately not built yet | `README.md` Status section |

## Commands

The repo root is **not** a module, so `go build ./...` fails there. `go.work` lists the
six Go modules.

```bash
# build everything
go build ./limelight-go/... ./limelight-go/limelightotel/... ./limelight-go/limelightzap/... \
         ./examples/go-log/... ./examples/go-log-zap/... ./examples/go-otel/...

# test — per module, each from the repo root
(cd limelight-go               && go vet ./... && go test ./...)
(cd limelight-go/limelightotel && go vet ./... && go test ./...)
(cd limelight-go/limelightzap  && go vet ./... && go test ./...)
(cd limelight-java             && mvn -q verify)   # nothing to build yet; compiles an empty package
```

A `//limelight:method` tag only does anything when the build goes through the shim:

```bash
go build -o /tmp/limelight ./limelight-go/cmd/limelight
cd examples/go-log && go run -toolexec="/tmp/limelight toolexec" .

# and for any test that asserts the tag fires
go test -toolexec="/tmp/limelight toolexec" ./...
```

`LIMELIGHT_VERBOSE=1` makes the shim report each method it instruments. Examples bind
`:8080` and `:6060` — check for a stale process before concluding something is broken.

<!-- Last reviewed 2026-09-20. Re-read this file every six months and delete what the
     codebase now says for itself; a rule nobody would violate is noise that buries the
     rules that matter. -->
