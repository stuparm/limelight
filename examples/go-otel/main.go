// Command example puts limelight's output into an OpenTelemetry pipeline.
//
// It is examples/go-log with a tracer added, and the base sampler set to
// NeverSample on purpose — so the terminal shows the one thing the otel
// integration is really for. With the switch off, a request produces a log line
// and no span at all. Turn limelight on for one project and that project's
// requests start producing spans, carrying the identity as attributes, while
// every other request still produces nothing.
//
// Spans print to stdout through stdouttrace, so there is no collector, no
// docker and nothing to install.
//
//	go build -o /tmp/limelight ../../limelight-go/cmd/limelight
//	go run -toolexec="/tmp/limelight toolexec" .
//
// Without the shim the directive is an ordinary comment and nothing is emitted.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"

	limelight "github.com/stuparm/limelight/limelight-go"
	_ "github.com/stuparm/limelight/limelight-go/control/auto"
	"github.com/stuparm/limelight/limelight-go/limelightotel"
)

type projectIDKey struct{}
type userIDKey struct{}

// ProjectIDFromContext is the sort of accessor over an unexported key that every
// codebase already has — and the reason a tag names a registered extractor
// instead of naming the key itself.
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

// tracing starts a span per request, the way otelhttp or otelgin would.
//
// It is registered *before* identity() on purpose: that is the ordering a real
// service has, and it is why limelightotel.Sampler is shaped the way it is.
// When this span is created nobody knows whose request it is, so limelight
// cannot ask for it to be sampled and the base sampler drops it. Everything
// created after identity() runs is what gets rescued.
func tracing(tracer oteltrace.Tracer) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, span := tracer.Start(c.Request.Context(), c.Request.Method+" "+c.FullPath())
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// identity lifts the caller's identity off the request and into the context.
// limelight reads it back out through the registered extractors.
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

// newRouter is separated from main so the test can drive it.
func newRouter(svc *Service, tracer oteltrace.Tracer) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), tracing(tracer), identity())
	r.POST("/api/things", func(c *gin.Context) {
		var body struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// A span the service would create anyway — a database write, a
		// downstream call. It is started after identity() has run, so this is
		// the one limelight can rescue, and the one its attributes land on.
		ctx, span := tracer.Start(c.Request.Context(), "Service.CreateThing")
		defer span.End()

		if err := svc.CreateThing(ctx, body.Name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"created": body.Name})
	})
	return r
}

// registerFields binds the extractors. Shared with the test.
func registerFields() {
	limelight.Register("projectID", ProjectIDFromContext, limelight.As("project.id"))
	limelight.Register("userID", UserIDFromContext, limelight.As("user.id"))
}

const (
	appAddr   = "localhost:8080"
	adminAddr = "localhost:6060"
)

func main() {
	registerFields()

	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatal(err)
	}
	tp := sdktrace.NewTracerProvider(
		// Syncer, not Batcher: a demo should print the span while you are still
		// looking at the request that produced it.
		sdktrace.WithSyncer(exporter),
		// The base sampler keeps nothing. Everything you see is limelight
		// asking for it.
		sdktrace.WithSampler(limelightotel.Sampler(sdktrace.NeverSample())),
	)
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)

	// Trace ids on every emitted event, whatever the backend...
	limelightotel.InstallTraceContext()
	// ...and the events going to the log and the live span at once.
	limelight.SetEmitter(limelight.MultiEmitter(
		limelight.NewLogEmitter(limelight.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil)))),
		limelightotel.New(),
	))

	go func() { log.Fatal(http.ListenAndServe(adminAddr, nil)) }()

	log.Printf(`limelight example — opentelemetry

  app   http://%s   (gin)
  admin http://%s   (limelight control)

The base sampler is NeverSample, so nothing is traced until limelight asks.

  1. a request while the switch is off — a log line, no span:
       curl -sS -XPOST http://%s/api/things \
         -H 'X-Project-ID: abc-123' -H 'X-User-ID: u-42' -d '{"name":"widget"}'

  2. trace project abc-123 for 30 seconds:
       curl -sS -XPOST http://%s/debug/limelight/enable \
         -d '{"version":0,"ttl":"30s","match":{"projectID":"abc-123"}}'

  3. repeat step 1 — now the log line carries a trace_id and a span is printed,
     with project.id and user.id as attributes and a "limelight" event on it.

  4. the same request as a different project stays untraced:
       curl -sS -XPOST http://%s/api/things \
         -H 'X-Project-ID: zzz-999' -H 'X-User-ID: u-7' -d '{"name":"gizmo"}'

`, appAddr, adminAddr, appAddr, adminAddr, appAddr)

	log.Fatal(newRouter(&Service{}, tp.Tracer("examples/go-otel")).Run(appAddr))
}
