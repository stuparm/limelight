// Package limelight is the Go runtime for limelight: an extractor registry, a
// TTL-bounded identity-targeted switch, the hot-path gate that generated code
// calls, and the Emitter contract that decides where events go.
//
// # This package is stdlib-only, on purpose
//
// Instrumentation is delivered by a -toolexec shim, and cmd/go resolves a
// package's dependencies before the shim runs. A package containing a tagged
// method must therefore already import this one — so whatever this package
// depends on lands in the dependency graph of every instrumented leaf package
// in the service.
//
// That is why Emitter is declared here rather than beside an implementation,
// why the only backend shipped alongside it is slog-backed, and why an OTel or
// zap backend belongs in its own package with its own go.mod. Those import
// this package; this package never imports them.
//
// Anything heavy that is not on that path — AST rewriting, static analysis —
// lives under cmd/ and internal/.
package limelight
