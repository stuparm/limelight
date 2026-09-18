# Design

## Positioning

Compile-time method tagging plus a runtime, TTL-bounded, **identity-targeted** switch that
emits product-semantic attributes (requestID / principalID / userID / projectID) into
existing traces and logs.

The tagging half is table stakes — otelc, orchestrion and go-instrument all do it. The
targeted, expiring switch is the product. The use case that justifies it is the one
support and on-call actually ask for: *trace everything for customer X for ten minutes*,
in a form another tool can join to real users.

## Requirements

1. Tag hundreds of methods; every tagged method has a context.
2. Emit only a **few named fields**, not the whole context. The tag says which.
3. The output must be joinable with product identity by a downstream tool.
4. **Not always on.** A per-service flag, flipped by a REST call with a TTL, live from
   that moment, auto-reverting.

## Two facts that drive everything

**Go has no annotations.** A tag is either a comment — invisible to the compiler, so it
silently does nothing without tooling — or real code. Nothing in between. This is why Go
needs a codemod and Java does not.

**`context.Context` is not enumerable.** `Value(k)` is a key lookup down a linked list;
there is no iterator. Logging "the whole context" is impossible without hacks, which is
fine, because requirement 2 only asks for named fields.

| Way at the data | Verdict |
|---|---|
| **Extractor registry** — user registers `func(ctx) (val, ok)` per field | **The design.** Portable across codebases. |
| `runtime/pprof.ForLabels` — the one std-lib-iterable bag on a ctx | Free bonus if a team uses `pprof.Do`. Almost nobody knows it exists. |
| reflection/`unsafe` walk of the `valueCtx` chain | Demo toy only. Breaks on custom Context impls; unexported `struct{}` keys don't stringify. Go's own position: ctx is not a data store ([go#33283](https://github.com/golang/go/issues/33283)). |

## Prior art

| Tool | Mechanism | Tag | Verdict |
|---|---|---|---|
| [otelc](https://opentelemetry.io/blog/2026/go-compile-time-instrumentation-v1/) | `-toolexec` | `//otelc:span` | v1 in 2026. Its *directive rule* injects a template into annotated bodies — the naive version of this idea, already shipped. |
| [orchestrion](https://github.com/DataDog/orchestrion) | `-toolexec` AST weave | `//dd:span` | Same concept, dd-trace-go-specific. Merging toward otelc via the OTel SIG. |
| [go-instrument](https://github.com/nikolaydubina/go-instrument) | source codemod | none, by design | Instruments every func with a ctx; ADR rejects skip-directives. Documents the lost-inlining cost. |
| [gowrap](https://github.com/hexdigest/gowrap) | template codegen | — | Interfaces only, but the template model is the one to copy. |
| [baggagecopy](https://pkg.go.dev/go.opentelemetry.io/contrib/processors/baggagecopy) | OTel span + log processor | filter func | Closest prior art to req 2. Copies filtered Baggage onto spans and logs, propagates cross-service. No typed `ctx.Value`, no per-method tags, no switch. |
| Datadog Dynamic Instrumentation | runtime probes | — | Closest prior art to req 4 — **auto-expiring** logpoints. Proprietary, agent-dependent, patches a running process. Ours is compiled in and merely gated. |
| OTel eBPF auto-instrumentation | uprobes | — | Hooks known library symbols; cannot see per-method tags or app context. |

[golang/go#69887](https://github.com/golang/go/issues/69887) proposes first-class
compile-time instrumentation in `cmd/go`. If it lands, anything built on `-toolexec`
rebases or dies. That was the argument that originally pointed at source rewrite; see
**Decisions** for why it lost and what keeps the rebase cheap.

## Decisions

**The tag names a registered extractor, never a raw ctx key.** Idiomatic keys are
unexported types in another package (`ctx.Value(requestIDKey{})`); generated code in a
different package cannot reference them. See [`../spec/field-contract.md`](../spec/field-contract.md).

**Field names map onto OTel semantic conventions.** `user.id`, not `userID`. This is what
makes requirement 3 real.

**Emitters are an interface, not templates** — revised 2026-09-18. The contract lives in
the root package, and an implementation ships beside it *iff* it costs no dependency
beyond stdlib; `NewLogEmitter` (slog) qualifies, otel and zap get their own packages and
their own `go.mod`. The template model was borrowed from gowrap, which generates
*decorators* that must match the user's own interfaces — but `Emitter` has one fixed
signature, so there is nothing shape-dependent to generate and `EmitterFunc` is the escape
hatch instead.

This placement is forced by `-toolexec`, not chosen for tidiness: every package holding a
tagged method must import the root package, so anything root depends on lands in the
dependency graph of every instrumented leaf package. An OTel-backed emitter in a
subpackage that root imported would put OpenTelemetry there — invisibly, until the first
non-stdlib backend landed.

**A `go vet` analyzer ships from day one.** A bare comment directive with no tooling in
the build is silently ignored — a user tags 200 methods and sees nothing. The analyzer is
what makes the tag trustworthy.

**`-toolexec`, not source rewrite**, for v0 — reversed 2026-09-17, having originally
decided the opposite. The tag takes effect inside an ordinary `go build` / `go run` and
no rewritten source is ever left on disk. The price is a build-graph constraint source
rewrite does not have: `cmd/go` computes each package's `importcfg` before the shim is
invoked and will not add to it, so **a package containing a tagged method must already
import the limelight runtime**. The shim detects this and fails with the import line to
add rather than emitting code that cannot compile. go#69887 above remains the standing
risk, and the reason the rewriter is a library the shim calls rather than a thing welded
to `-toolexec`.

## Costs, stated rather than discovered

- **Inlining: a budget cost, not the unconditional loss first assumed.** Measured on
  go1.27.1/arm64, the injected gate takes a method from inline cost 5 to 69 against the
  inliner's budget of 80 — so a method whose own cost is ≤16 still inlines and anything
  larger falls out. This holds *only* while emission is entry-only. Entry + exit needs a
  `defer`, and the `defer` is what makes the loss permanent and unconditional; that
  trade is the real content of the entry-vs-exit question in
  [`../spec/event-schema.md`](../spec/event-schema.md).
- **~1.2 ns per tagged call when off**, zero allocations (M1 Pro). Reaching that is why
  generated code is `if limelight.On() { limelight.Emit(...) }` rather than a bare
  `Emit`: `Emit` is variadic, so an unguarded call makes the caller build the field-name
  slice before `Emit` can decide it has nothing to do — 4.9 ns against 1.2 ns, on every
  tagged call, for the ~100% of calls where the switch is off. `On` is a single atomic
  load and inlines into the caller. For the TTL, never call `time.Now()` per invocation —
  an `atomic.Pointer[config]` that a timer nils.
- **A REST toggle behind a load balancer enables one pod.** v0 is honest per-pod scope.
- **Security:** the endpoint turns on emission of user identifiers and can flood a logging
  bill. Authenticate, rate-limit, cap the TTL server-side, audit.
- **Cardinality:** fine as span attributes and log fields, never as metric labels.

## Open: what the fields ride on

Decide by prototype on one real service, not by argument.

- **A: extractors → OTel Baggage → `baggagecopy`.** Gets most of requirements 2 and 3 for
  free, including cross-service propagation, and leaves only the directive, the analyzer
  and the switch to build. Baggage is also the one context abstraction that is identical
  in every language — which matters for the Java SDK. Counter-argument: baggage propagates
  over the wire to third parties; a user id in baggage is an egress concern that
  `ctx.Value` is not.
- **B: a custom emitter** reading extractors directly. No egress surprise, more code.

## Don't invent the control plane

[OpAMP](https://opentelemetry.io/docs/specs/opamp/) is OTel's remote agent-configuration
protocol. [OpenFeature](https://github.com/open-feature/protocol)'s *evaluation context* is
literally "named fields about user/session used for targeting rules" — "is limelight on
for this user?" is a flag evaluation with targeting. The only piece flags don't give you
is the TTL.

## Other languages

| Language | Native tagging | Notes |
|---|---|---|
| **Go** | none — hence the codemod | The only language that needs one. |
| **Java** | `@WithSpan` + `@SpanAttribute`, honored by the OTel javaagent | Args can already be promoted to span attributes. Under Spring these go through AOP proxies: Spring-managed beans, external calls only, **not** internal self-calls. |
| **Rust** | `#[tracing::instrument]` with `fields(...)` | Async needs `.instrument()`; `tracing-opentelemetry` bridges to OTel. |
| **Python** | decorators + `contextvars` | — |

Which is why the portable assets are the three in [`../spec/`](../spec/), and why the name
carries no `ctx` in it: Java and Rust have no context parameter.
