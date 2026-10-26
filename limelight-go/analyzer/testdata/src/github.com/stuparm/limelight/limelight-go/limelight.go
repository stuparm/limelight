// Package limelight stands in for the runtime. The analyzer only ever looks for
// its import path.
package limelight

// Stub gives an importing file something to refer to.
var Stub = true

// The shim never instruments the runtime, so the analyzer must not judge a tag
// in it either: this malformed one is never reported.
//
//limelight:method
func neverJudged() {}
