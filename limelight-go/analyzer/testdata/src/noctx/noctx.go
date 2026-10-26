package noctx

import (
	"context"

	_ "github.com/stuparm/limelight/limelight-go"
)

//limelight:method fields:"projectID"
func NoCtx(name string) {} // want `//limelight:method on NoCtx, which takes no context.Context`

//limelight:method fields:"projectID"
func Discarded(_ context.Context) {} // want `on Discarded, which takes no context.Context`

// Ctx is context.Context to the type checker, but the rewriter matches the
// spelling context.Context, so the shim skips a method taking it. The analyzer
// must agree with the shim, not with the type checker.
type Ctx = context.Context

//limelight:method fields:"projectID"
func Aliased(ctx Ctx) {} // want `on Aliased, which takes no context.Context`

//limelight:method fields:"projectID"
func NoBody(ctx context.Context) // want `on NoBody, which has no body`

// want +2 `not attached to a function declaration`
//
//limelight:method fields:"projectID"
type T struct{}

type I interface {
	// want +1 `not attached to a function declaration`
	//limelight:method fields:"projectID"
	M(ctx context.Context)
}
