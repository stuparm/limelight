package testonly

import (
	"context"
	"testing"

	_ "github.com/stuparm/limelight/limelight-go"
)

// helper is compiled only alongside this file, so this file's import covers it.
//
//limelight:method fields:"projectID"
func helper(ctx context.Context) {}

func TestF(t *testing.T) {
	F(context.Background())
	helper(context.Background())
}
