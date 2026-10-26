// Package ctxonly's one tag cannot be instrumented, so the shim never asks for
// the runtime import — and neither may the analyzer.
package ctxonly

//limelight:method fields:"projectID"
func C(name string) {} // want `takes no context.Context`
