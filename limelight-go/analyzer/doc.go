// Package analyzer is a go/analysis pass that fails the build on a
// //limelight:method directive that cannot work — a method with no
// context.Context, or a fields:"..." name that is never Register'd.
//
// Exported so teams can wire it into their own vet binary. A bare comment
// directive is silently ignored by the Go compiler, so this is what makes the
// tag trustworthy; it ships from day one.
package analyzer
