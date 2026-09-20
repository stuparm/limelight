package limelight

import (
	"context"
	"sync/atomic"
)

// emitterHolder boxes the interface so it fits in an atomic.Pointer.
type emitterHolder struct{ e Emitter }

var emitter atomic.Pointer[emitterHolder]

// SetEmitter installs the backend that tagged methods write to. Until it is
// called, events are discarded — an instrumented binary that never configured
// an emitter stays silent rather than writing somewhere surprising.
//
// A nil e restores that discarding behaviour.
func SetEmitter(e Emitter) {
	if e == nil {
		e = DiscardEmitter
	}
	emitter.Store(&emitterHolder{e: e})
}

// Emit is the hot-path gate. Generated code calls it as the first statement of
// every tagged method; nothing else should.
//
// When the switch is off this is one atomic load and a branch — no time.Now,
// no lock, no allocation. The names are the field names from the directive's
// fields:"..." list.
func Emit(ctx context.Context, method string, names ...string) {
	a := current.Load()
	// Targeting first: a call that does not match the active identity must not
	// pay for extraction of the fields it would have emitted.
	if !matches(a, ctx) {
		return
	}

	h := emitter.Load()
	if h == nil {
		return
	}

	fields := make(map[string]string, len(names))
	for _, name := range names {
		f, ok := lookup(name)
		if !ok {
			// A directive named a field nobody registered. The analyzer is
			// what catches this at build time; at run time, skip it rather
			// than emit a half-event under a misleading name.
			continue
		}
		v, ok := f.extract(ctx)
		if !ok {
			continue
		}
		fields[f.attrKey] = v
	}

	ev := Event{
		Version: EventVersion,
		Method:  method,
		Fields:  fields,
	}
	if t := traceContext.Load(); t != nil {
		ev.TraceID, ev.SpanID = t.f(ctx)
	}
	h.e.Emit(ctx, ev)
}

// Matches reports whether ctx satisfies the active targeting — the same
// question Emit asks before emitting.
//
// It is exported for a sampler: "is limelight watching this request?" is the
// decision an OpenTelemetry Sampler has to make at span creation, and it must
// be the identical question, or targeting and sampling would disagree about
// which requests matter.
func Matches(ctx context.Context) bool {
	return matches(current.Load(), ctx)
}

func matches(a *active, ctx context.Context) bool {
	if a == nil {
		return false
	}
	// matchAll skips extraction entirely — there is nothing to disagree with.
	if a.matchAll {
		return true
	}
	for name, want := range a.match {
		f, ok := lookup(name)
		if !ok {
			return false
		}
		got, ok := f.extract(ctx)
		if !ok || got != want {
			return false
		}
	}
	return true
}

// On reports whether the switch is currently on.
//
// It exists so generated code can pay the off-path cost without building an
// argument list: On is one atomic load and inlines into the caller, whereas a
// call to Emit forces the variadic field-name slice to be constructed first,
// however quickly Emit then returns.
func On() bool { return current.Load() != nil }
