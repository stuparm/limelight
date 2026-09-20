// Command example is examples/go-log with one line changed: the emitter.
//
// Everything else — the gin API, the identity middleware, the tagged method,
// the admin listener — is identical, which is the point. Swapping the backend
// does not touch the tag, the registry, the switch or the control endpoint.
//
// The two never collide — they are different listeners on different ports, and
// the only thing they share is the process.
//
// The //limelight:method directive below only does something when the build
// goes through the shim:
//
//	go build -o /tmp/limelight ../../limelight-go/cmd/limelight
//	go run -toolexec="/tmp/limelight toolexec" .
//
// Run it with a plain `go run .` and the tag is an ordinary comment: the API
// works and emits nothing.
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	limelight "github.com/stuparm/limelight/limelight-go"
	"github.com/stuparm/limelight/limelight-go/limelightzap"
	// Registers /debug/limelight/ on http.DefaultServeMux, which the admin
	// listener below serves. Swap this for control.Mount(mux) if you want the
	// routes on a mux you build yourself.
	_ "github.com/stuparm/limelight/limelight-go/control/auto"
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
	log.Println("created", name)
	return nil
}

// identity is the middleware every service already has in some form: it lifts
// the caller's identity off the request and into the context. limelight reads
// it back out through the extractors registered in main.
func identity() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if v := c.GetHeader("X-Project-ID"); v != "" {
			ctx = context.WithValue(ctx, projectIDKey{}, v)
		}
		if v := c.GetHeader("X-User-ID"); v != "" {
			ctx = context.WithValue(ctx, userIDKey{}, v)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

const (
	appAddr   = "localhost:8080" // the product's API
	adminAddr = "localhost:6060" // limelight's control endpoint, pprof-style
)

func main() {
	limelight.Register("projectID", ProjectIDFromContext, limelight.As("project.id"))
	limelight.Register("userID", UserIDFromContext, limelight.As("user.id"))
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = logger.Sync() }()
	limelight.SetEmitter(limelightzap.New(logger))

	svc := &Service{}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), identity())
	r.POST("/api/things", func(c *gin.Context) {
		var body struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := svc.CreateThing(c.Request.Context(), body.Name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"created": body.Name})
	})

	// The admin listener. Separate port on purpose: the control endpoint turns
	// on emission of user identifiers, so production binds this to loopback or
	// an internal interface and never routes it from outside.
	go func() { log.Fatal(http.ListenAndServe(adminAddr, nil)) }()

	log.Printf(`limelight example

  app   http://%s   (gin)
  admin http://%s   (limelight control)

  create a thing as project abc-123:
    curl -sS -XPOST http://%s/api/things \
      -H 'X-Project-ID: abc-123' -H 'X-User-ID: u-42' -d '{"name":"widget"}'

  trace project abc-123 for 30 seconds:
    curl -sS -XPOST http://%s/debug/limelight/enable \
      -d '{"version":0,"ttl":"30s","match":{"projectID":"abc-123"}}'

  or, when you do not know the identity yet, trace everything for 10 seconds:
    curl -sS -XPOST http://%s/debug/limelight/enable \
      -d '{"version":0,"ttl":"10s","match_all":true}'

Requests from any other project run through the same tagged method and stay silent.
`, appAddr, adminAddr, appAddr, adminAddr, adminAddr)

	log.Fatal(r.Run(appAddr))
}
