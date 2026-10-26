// Package testonly imports the runtime from a test file only: go test
// -toolexec builds it, go build -toolexec refuses it.
package testonly

import "context"

// want +2 `does not import github.com/stuparm/limelight/limelight-go`
//
//limelight:method fields:"projectID"
func F(ctx context.Context) {}
