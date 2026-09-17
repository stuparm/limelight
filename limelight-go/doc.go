// Package limelight is the Go runtime for limelight: an extractor registry, a
// TTL-bounded identity-targeted switch, and the hot-path gate that generated
// code calls.
//
// It is deliberately dependency-light: user services import this package, so
// anything heavy (AST rewriting, static analysis) lives in cmd/ and analyzer/.
package limelight
