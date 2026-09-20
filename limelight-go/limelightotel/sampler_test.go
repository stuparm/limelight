package limelightotel_test

import (
	"context"
	"testing"

	"github.com/stuparm/limelight/limelight-go/limelightotel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func names(spans []sdktrace.ReadOnlySpan) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.Name())
	}
	return out
}

// A root span created with the identity already on the context: the sampler can
// see it, so the whole trace is kept despite a base sampler that keeps nothing.
func TestRootSpanIsForceSampledWhenTargeted(t *testing.T) {
	target(t)
	tp, sr := recorder(limelightotel.Sampler(sdktrace.NeverSample()))

	_, span := tp.Tracer("t").Start(ctxWith("abc-123"), "GET /api/things")
	span.End()

	if got := names(sr.Ended()); len(got) != 1 {
		t.Fatalf("spans = %v, want the targeted root span to be kept", got)
	}
}

func TestUntargetedRootSpanFollowsTheBaseSampler(t *testing.T) {
	target(t)
	tp, sr := recorder(limelightotel.Sampler(sdktrace.NeverSample()))

	_, span := tp.Tracer("t").Start(ctxWith("zzz-999"), "GET /api/things")
	span.End()

	if got := names(sr.Ended()); len(got) != 0 {
		t.Errorf("spans = %v, want none — a request nobody targeted was force-sampled", got)
	}
}

// The real arrangement, and the reason this sampler is shaped the way it is.
//
// otelhttp creates the server span before the service's identity middleware
// runs, so at that moment nobody knows whose request it is and the base sampler
// drops it. The identity arrives afterwards. Every span created from then on
// must override the inherited "not sampled" — which is what
// WithLocalParentNotSampled is for.
func TestSpansAfterTheIdentityArrivesAreRescued(t *testing.T) {
	target(t)
	tp, sr := recorder(limelightotel.Sampler(sdktrace.NeverSample()))
	tracer := tp.Tracer("t")

	// 1. otelhttp starts the server span. No identity on the context yet.
	rootCtx, root := tracer.Start(context.Background(), "GET /api/things")
	if root.SpanContext().IsSampled() {
		t.Fatal("the root span was sampled; this test no longer reproduces the ordering problem")
	}

	// 2. the service's identity middleware runs.
	identified := context.WithValue(rootCtx, projectKey{}, "abc-123")

	// 3. the tagged method's span.
	_, child := tracer.Start(identified, "billing.Service.CreateThing")
	child.End()
	root.End()

	got := names(sr.Ended())
	if len(got) != 1 || got[0] != "billing.Service.CreateThing" {
		t.Fatalf("spans = %v, want just the tagged method — the rescue did not fire", got)
	}
	if !child.SpanContext().IsSampled() {
		t.Error("the rescued span is not marked sampled, so downstream services will not honour it")
	}
	// The trace is partial by construction: the HTTP parent is gone, the tagged
	// work is present. That is the documented trade.
	for _, s := range sr.Ended() {
		if s.Name() == "GET /api/things" {
			t.Error("the root span came back; the sampler is doing more than claimed")
		}
	}
}

// Same shape, but limelight is not targeting this request: the inherited "not
// sampled" must stand, or the sampler would leak every trace in the process.
func TestUnrelatedRequestsAreNotRescued(t *testing.T) {
	target(t)
	tp, sr := recorder(limelightotel.Sampler(sdktrace.NeverSample()))
	tracer := tp.Tracer("t")

	rootCtx, root := tracer.Start(context.Background(), "GET /api/things")
	_, child := tracer.Start(context.WithValue(rootCtx, projectKey{}, "zzz-999"), "billing.Service.CreateThing")
	child.End()
	root.End()

	if got := names(sr.Ended()); len(got) != 0 {
		t.Errorf("spans = %v, want none", got)
	}
}

// With the switch off entirely, the sampler must be invisible.
func TestSwitchOffLeavesTheBaseSamplerAlone(t *testing.T) {
	tpAlways, srAlways := recorder(limelightotel.Sampler(sdktrace.AlwaysSample()))
	_, s1 := tpAlways.Tracer("t").Start(ctxWith("abc-123"), "op")
	s1.End()
	if got := len(srAlways.Ended()); got != 1 {
		t.Errorf("AlwaysSample base kept %d spans, want 1", got)
	}

	tpNever, srNever := recorder(limelightotel.Sampler(sdktrace.NeverSample()))
	_, s2 := tpNever.Tracer("t").Start(ctxWith("abc-123"), "op")
	s2.End()
	if got := len(srNever.Ended()); got != 0 {
		t.Errorf("NeverSample base kept %d spans, want 0", got)
	}
}
