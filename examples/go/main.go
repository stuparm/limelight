// Command example is a tiny service that exercises the whole limelight loop:
// register extractors, tag a method, flip the switch against the running
// process, watch the fields appear and then stop when the TTL expires.
//
// The //limelight:method directive below only does something when the build
// goes through the shim:
//
//	go build -o /tmp/limelight ../../limelight-go/cmd/limelight
//	go run -toolexec="/tmp/limelight toolexec" .
//
// Run it with a plain `go run .` and the tag is an ordinary comment: the
// service works, and emits nothing.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	limelight "github.com/stuparm/limelight/limelight-go"
	"github.com/stuparm/limelight/limelight-go/control"
)

type projectIDKey struct{}
type userIDKey struct{}

// ProjectIDFromContext is the sort of accessor over an unexported key that every
// codebase already has — and exactly the reason a tag names a registered
// extractor instead of naming the key itself. Generated code lives in a
// different package and could never write ctx.Value(projectIDKey{}).
func ProjectIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(projectIDKey{}).(string)
	return v, ok
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey{}).(string)
	return v, ok
}

// Service is the thing under observation.
type Service struct{}

//limelight:method fields:"projectID,userID"
func (s *Service) CreateThing(ctx context.Context, name string) error {
	fmt.Println("created", name)
	return nil
}

const addr = "localhost:8080"

func main() {
	limelight.Register("projectID", ProjectIDFromContext, limelight.As("project.id"))
	limelight.Register("userID", UserIDFromContext, limelight.As("user.id"))
	limelight.SetEmitter(limelight.NewLogEmitter(
		limelight.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil))),
	))

	mux := http.NewServeMux()
	mux.Handle(control.Prefix, control.Handler())
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	fmt.Fprintf(os.Stderr, `limelight example on http://%s

  trace project abc-123 for 10 seconds:
    curl -sS -XPOST http://%s%senable \
      -d '{"version":0,"ttl":"10s","match":{"projectID":"abc-123"}}'

  stop early:
    curl -sS -XPOST http://%s%sdisable

Two requests go through CreateThing every second, one for project abc-123 and
one for zzz-999. Only the targeted one should ever emit.

`, addr, addr, control.Prefix, addr, control.Prefix)

	svc := &Service{}
	ctxFor := func(project, user string) context.Context {
		ctx := context.WithValue(context.Background(), projectIDKey{}, project)
		return context.WithValue(ctx, userIDKey{}, user)
	}

	for range time.Tick(time.Second) {
		if err := svc.CreateThing(ctxFor("abc-123", "u-42"), "widget"); err != nil {
			log.Fatal(err)
		}
		if err := svc.CreateThing(ctxFor("zzz-999", "u-7"), "gizmo"); err != nil {
			log.Fatal(err)
		}
	}
}
