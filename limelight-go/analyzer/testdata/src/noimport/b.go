package noimport

import "context"

// want +2 `non-test file`
//
//limelight:method fields:"projectID"
func B(ctx context.Context) {}
