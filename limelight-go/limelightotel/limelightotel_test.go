package limelightotel_test

import (
	"context"
	"testing"
	"time"

	limelight "github.com/stuparm/limelight/limelight-go"
	"github.com/stuparm/limelight/limelight-go/limelightotel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type projectKey struct{}

func ctxWith(project string) context.Context {
	return context.WithValue(context.Background(), projectKey{}, project)
}

func init() {
	limelight.Register("projectID", func(ctx context.Context) (string, bool) {
		v, ok := ctx.Value(projectKey{}).(string)
		return v, ok
	}, limelight.As("project.id"))
}

// target arms limelight for abc-123 for the duration of the test.
func target(t *testing.T) {
	t.Helper()
	if _, err := limelight.Enable(limelight.Config{
		TTL:   time.Minute,
		Match: map[string]string{"projectID": "abc-123"},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { limelight.Disable() })
}

func recorder(sampler sdktrace.Sampler) (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler), sdktrace.WithSpanProcessor(sr))
	return tp, sr
}

func TestTraceContext(t *testing.T) {
	if id, span := limelightotel.TraceContext(context.Background()); id != "" || span != "" {
		t.Errorf("no span should give empty ids, got %q / %q", id, span)
	}

	tp, _ := recorder(sdktrace.AlwaysSample())
	ctx, span := tp.Tracer("t").Start(context.Background(), "op")
	defer span.End()

	traceID, spanID := limelightotel.TraceContext(ctx)
	if traceID != span.SpanContext().TraceID().String() || spanID != span.SpanContext().SpanID().String() {
		t.Errorf("got %q / %q, want %v / %v", traceID, spanID, span.SpanContext().TraceID(), span.SpanContext().SpanID())
	}
}

// The hook is what gives slog and zap lines their trace ids.
func TestInstallTraceContextReachesEveryEmitter(t *testing.T) {
	target(t)
	limelightotel.InstallTraceContext()
	t.Cleanup(func() { limelight.SetTraceContextFunc(nil) })

	var got limelight.Event
	limelight.SetEmitter(limelight.EmitterFunc(func(_ context.Context, ev limelight.Event) { got = ev }))
	t.Cleanup(func() { limelight.SetEmitter(nil) })

	tp, _ := recorder(sdktrace.AlwaysSample())
	ctx, span := tp.Tracer("t").Start(ctxWith("abc-123"), "op")
	defer span.End()

	limelight.Emit(ctx, "billing.Service.CreateThing", "projectID")

	if got.TraceID != span.SpanContext().TraceID().String() {
		t.Errorf("TraceID = %q, want %v", got.TraceID, span.SpanContext().TraceID())
	}
	if got.SpanID == "" {
		t.Error("SpanID is empty")
	}
}

func TestEmitterWritesAttributesAndEvent(t *testing.T) {
	tp, sr := recorder(sdktrace.AlwaysSample())
	ctx, span := tp.Tracer("t").Start(context.Background(), "op")

	limelightotel.New().Emit(ctx, limelight.Event{
		Version: limelight.EventVersion,
		Method:  "billing.Service.CreateThing",
		Fields:  map[string]string{"project.id": "abc-123", "user.id": "u-42"},
	})
	span.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("got %d spans, want 1", len(ended))
	}

	attrs := map[string]string{}
	for _, kv := range ended[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	if attrs["project.id"] != "abc-123" || attrs["user.id"] != "u-42" {
		t.Errorf("span attributes = %v", attrs)
	}

	events := ended[0].Events()
	if len(events) != 1 || events[0].Name != limelightotel.EventName {
		t.Fatalf("events = %+v, want one named %q", events, limelightotel.EventName)
	}
	var method string
	for _, kv := range events[0].Attributes {
		if string(kv.Key) == limelightotel.MethodAttr {
			method = kv.Value.AsString()
		}
	}
	if method != "billing.Service.CreateThing" {
		t.Errorf("%s = %q", limelightotel.MethodAttr, method)
	}
}

// Several tagged methods in one span must not overwrite each other — which is
// why the per-method record is an event and not an attribute.
func TestSeveralMethodsInOneSpan(t *testing.T) {
	tp, sr := recorder(sdktrace.AlwaysSample())
	ctx, span := tp.Tracer("t").Start(context.Background(), "op")

	e := limelightotel.New()
	e.Emit(ctx, limelight.Event{Method: "billing.Service.A", Fields: map[string]string{"project.id": "abc-123"}})
	e.Emit(ctx, limelight.Event{Method: "billing.Service.B", Fields: map[string]string{"project.id": "abc-123"}})
	span.End()

	if got := len(sr.Ended()[0].Events()); got != 2 {
		t.Errorf("got %d events, want 2 — one method overwrote the other", got)
	}
}

func TestEmitterIsQuietWhenNotRecording(t *testing.T) {
	tp, sr := recorder(sdktrace.NeverSample())
	ctx, span := tp.Tracer("t").Start(context.Background(), "op")
	limelightotel.New().Emit(ctx, limelight.Event{Method: "billing.Service.CreateThing"})
	span.End()

	if n := len(sr.Ended()); n != 0 {
		t.Errorf("got %d recorded spans, want 0", n)
	}
	// And it must not panic on a context with no span at all.
	limelightotel.New().Emit(context.Background(), limelight.Event{Method: "x"})
}
