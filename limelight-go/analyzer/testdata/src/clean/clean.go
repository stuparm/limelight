// Package clean holds what the analyzer must stay silent about: valid tags, and
// prose about limelight that has the shape of a mistyped directive.
package clean

import (
	"context"

	limelight "github.com/stuparm/limelight/limelight-go"
)

var _ = limelight.Stub

type Service struct{}

// CreateThing is tagged.
//
// limelight: method tagging is entry-only
// Limelight: methods are tagged at compile time
// limelight: skip this handler, it has no identity
// limelight: type ladder is not built yet
// A plain go test compiles //limelight:method as an ordinary comment.
//
//	//limelight:method fields:"projectID"
//
//limelight:method fields:"projectID,userID"
func (s *Service) CreateThing(ctx context.Context, name string) error { return nil }

//limelight:method fields:"projectID"
func Plain(c context.Context, n int) {}

type Box[T any] struct{}

//limelight:method fields:"projectID"
func (b *Box[T]) Get(ctx context.Context) {}

/* limelight: method tagging is entry-only */

//go:noinline
//nolint:errcheck
func Other() {}
