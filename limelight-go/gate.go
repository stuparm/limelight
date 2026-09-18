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
	if a == nil {
		return
	}

	// Targeting first: a call that does not match the active identity must not
	// pay for extraction of the fields it would have emitted.
	for name, want := range a.match {
		f, ok := lookup(name)
		if !ok {
			return
		}
		got, ok := f.extract(ctx)
		if !ok || got != want {
			return
		}
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

	h.e.Emit(ctx, Event{
		Version: EventVersion,
		Method:  method,
		Fields:  fields,
	})
}

// On reports whether the switch is currently on.
//
// It exists so generated code can pay the off-path cost without building an
// argument list: On is one atomic load and inlines into the caller, whereas a
// call to Emit forces the variadic field-name slice to be constructed first,
// however quickly Emit then returns.
func On() bool { return current.Load() != nil }
