package limelightotel

import (
	limelight "github.com/stuparm/limelight/limelight-go"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Sampler wraps base so that a request limelight is targeting is sampled even
// when it otherwise would not be.
//
//	tp := sdktrace.NewTracerProvider(
//		sdktrace.WithSampler(limelightotel.Sampler(sdktrace.TraceIDRatioBased(0.01))),
//	)
//
// Without this, "trace everything for customer X" is only true for the fraction
// of X's requests the base sampler happened to keep — at 1% sampling, one in a
// hundred. Nothing reports that the rest were lost.
//
// # What it can and cannot do
//
// A sampling decision is made when a span is created, from the context as it
// exists at that moment. limelight's targeting reads an identity that a service
// puts on the context in middleware — and in the usual otelhttp arrangement the
// server span is created *before* that middleware runs. So the root span of a
// request is generally past saving: at the moment it was sampled, nobody knew
// whose request it was.
//
// What this sampler therefore overrides is the *inherited* decision. Every span
// created after the identity is known — which is every span the tagged methods
// run inside — is force-sampled, even though its parent was not. The result is
// a partial trace: the tagged work is present, its HTTP parent is not.
//
// A service that can extract identity before its tracing middleware gets whole
// traces instead, with no change to this sampler: the root decision then has
// the identity available and base is never consulted for those requests.
func Sampler(base sdktrace.Sampler) sdktrace.Sampler {
	rescue := rescueSampler{base: base}
	return sdktrace.ParentBased(
		targetedSampler{base: base},
		sdktrace.WithLocalParentNotSampled(rescue),
		sdktrace.WithRemoteParentNotSampled(rescue),
	)
}

// targetedSampler handles spans with no parent: force-sample when limelight is
// targeting this request, otherwise defer to base.
type targetedSampler struct{ base sdktrace.Sampler }

func (s targetedSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	if limelight.Matches(p.ParentContext) {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Tracestate: tracestate(p),
		}
	}
	return s.base.ShouldSample(p)
}

func (s targetedSampler) Description() string {
	return "limelight{" + s.base.Description() + "}"
}

// rescueSampler handles spans whose parent was not sampled. Inheriting that
// "no" is the default; this overrides it for a targeted request, which is what
// recovers the tagged work when the root span was created before the identity
// was known.
type rescueSampler struct{ base sdktrace.Sampler }

func (s rescueSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	if limelight.Matches(p.ParentContext) {
		return sdktrace.SamplingResult{
			Decision:   sdktrace.RecordAndSample,
			Tracestate: tracestate(p),
		}
	}
	return sdktrace.SamplingResult{
		Decision:   sdktrace.Drop,
		Tracestate: tracestate(p),
	}
}

func (s rescueSampler) Description() string {
	return "limelightRescue{" + s.base.Description() + "}"
}

// tracestate carries the parent's tracestate through, so a decision made here
// propagates to downstream services over W3C trace context rather than being
// remade — and disagreed with — in each one.
func tracestate(p sdktrace.SamplingParameters) trace.TraceState {
	return trace.SpanContextFromContext(p.ParentContext).TraceState()
}
