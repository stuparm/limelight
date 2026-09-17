// Command example is a tiny service that exercises the whole limelight loop:
// register extractors, tag a method, flip the switch, watch the fields appear
// and then stop when the TTL expires.
//
// It panics until the SDK is implemented. That is the point — this is the
// target that development runs against.
package main

import (
	"context"
	"fmt"

	limelight "github.com/stuparm/limelight/limelight-go"
)

type projectIDKey struct{}

// ProjectIDFromContext is the sort of accessor over an unexported key that every
// codebase already has — and exactly the reason a tag names a registered
// extractor instead of naming the key itself.
func ProjectIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(projectIDKey{}).(string)
	return v, ok
}

// Service is the thing under observation.
type Service struct{}

//limelight:method fields:"projectID"
func (s *Service) CreateThing(ctx context.Context, name string) error {
	fmt.Println("created", name)
	return nil
}

func main() {
	limelight.Register("projectID", ProjectIDFromContext, limelight.As("project.id"))

	// TODO: mount control.Handler() on /debug/limelight/ and serve, so the
	// switch can be flipped against a running process.

	ctx := context.WithValue(context.Background(), projectIDKey{}, "abc-123")
	if err := (&Service{}).CreateThing(ctx, "widget"); err != nil {
		panic(err)
	}
}
