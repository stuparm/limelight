// Package noimport has tagged methods and never imports the runtime, so the
// shim refuses to build it.
package noimport

import "context"

// want +2 `package noimport has tagged methods but does not import github.com/stuparm/limelight/limelight-go`
//
//limelight:method fields:"projectID"
func A(ctx context.Context) {}

// A2 gets no diagnostic of its own: the import is reported once per file.
//
//limelight:method fields:"projectID"
func A2(ctx context.Context) {}
