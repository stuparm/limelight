package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	limelight "github.com/stuparm/limelight/limelight-go"
	"github.com/stuparm/limelight/limelight-go/limelightotel"
)

// events collects what the tag produced.
type events struct {
	mu   sync.Mutex
	seen []limelight.Event
}

func (e *events) Emit(_ context.Context, ev limelight.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seen = append(e.seen, ev)
}

func (e *events) drain() []limelight.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.seen
	e.seen = nil
	return out
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.Name())
	}
	return out
}

func attrsOf(s sdktrace.ReadOnlySpan) map[string]string {
	out := map[string]string{}
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.AsString()
	}
	return out
}

// TestOtelIntegration drives the real router and asserts the whole chain: the
// tag fires, the event carries a trace id, the identity lands on a span as
// attributes, and a request nobody targeted produces no span at all.
//
// It only means anything when the test binary is built through the shim:
//
//	go build -o /tmp/limelight ../../limelight-go/cmd/limelight
//	go test -toolexec="/tmp/limelight toolexec" ./...
//
// A plain `go test` compiles //limelight:method as an ordinary comment, so the
// tag never fires. The first subtest fails loudly in that case rather than
// passing while proving nothing.
func TestOtelIntegration(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(limelightotel.Sampler(sdktrace.NeverSample())),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	registerFields()
	limelightotel.InstallTraceContext()
	collected := &events{}
	limelight.SetEmitter(limelight.MultiEmitter(collected, limelightotel.New()))
	t.Cleanup(func() {
		limelight.Disable()
		limelight.SetEmitter(nil)
		limelight.SetTraceContextFunc(nil)
	})

	router := newRouter(&Service{}, tp.Tracer("test"))
	post := func(project, user, name string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/things", strings.NewReader(`{"name":"`+name+`"}`))
		req.Header.Set("X-Project-ID", project)
		req.Header.Set("X-User-ID", user)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /api/things: %d %s", rec.Code, rec.Body)
		}
	}

	t.Run("switch off: no events, no spans", func(t *testing.T) {
		post("abc-123", "u-42", "widget")
		if got := collected.drain(); len(got) != 0 {
			t.Errorf("emitted %d events with the switch off", len(got))
		}
		if got := spanNames(recorder.Ended()); len(got) != 0 {
			t.Errorf("spans = %v, want none — the base sampler keeps nothing", got)
		}
	})

	if _, err := limelight.Enable(limelight.Config{
		TTL:   time.Minute,
		Match: map[string]string{"projectID": "abc-123"},
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("targeted: event carries a trace id, span carries the identity", func(t *testing.T) {
		post("abc-123", "u-42", "widget")

		got := collected.drain()
		if len(got) == 0 {
			t.Fatal("the tag never fired.\n" +
				"This test is meaningless without the -toolexec shim. Run it as:\n" +
				"  go build -o /tmp/limelight ../../limelight-go/cmd/limelight\n" +
				`  go test -toolexec="/tmp/limelight toolexec" ./...`)
		}
		if len(got) != 1 {
			t.Fatalf("got %d events, want 1", len(got))
		}
		if got[0].Method != "main.Service.CreateThing" {
			t.Errorf("Method = %q", got[0].Method)
		}
		if got[0].TraceID == "" || got[0].SpanID == "" {
			t.Error("the event carries no trace ids; InstallTraceContext did not take effect")
		}
		if got[0].Fields["project.id"] != "abc-123" || got[0].Fields["user.id"] != "u-42" {
			t.Errorf("Fields = %v", got[0].Fields)
		}

		ended := recorder.Ended()
		names := spanNames(ended)
		// The server span was created before identity() ran, so it could not be
		// rescued. The one started afterwards was.
		if len(ended) != 1 || names[0] != "Service.CreateThing" {
			t.Fatalf("spans = %v, want exactly [Service.CreateThing]", names)
		}
		attrs := attrsOf(ended[0])
		if attrs["project.id"] != "abc-123" || attrs["user.id"] != "u-42" {
			t.Errorf("span attributes = %v", attrs)
		}
		spanEvents := ended[0].Events()
		if len(spanEvents) != 1 || spanEvents[0].Name != limelightotel.EventName {
			t.Errorf("span events = %+v, want one named %q", spanEvents, limelightotel.EventName)
		}
	})

	t.Run("untargeted: nothing", func(t *testing.T) {
		before := len(recorder.Ended())
		post("zzz-999", "u-7", "gizmo")
		if got := collected.drain(); len(got) != 0 {
			t.Errorf("emitted %d events for an untargeted project", len(got))
		}
		if after := len(recorder.Ended()); after != before {
			t.Errorf("recorded %d new spans for an untargeted project", after-before)
		}
	})
}
