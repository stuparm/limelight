package limelight

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Option configures a registered field.
type Option func(*field)

type field struct {
	name    string
	attrKey string
	extract func(context.Context) (string, bool)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]*field{}
)

// Register binds a field name — the name used in a //limelight:method
// fields:"..." directive — to a function that pulls it out of a context.
//
// The tag never names a raw context key: idiomatic keys are unexported types
// owned by another package, and generated code cannot reference them. The
// registry is what makes limelight portable across codebases.
//
//	limelight.Register("projectID", project.IDFromContext, limelight.As("project.id"))
//
// Register is safe to call concurrently, but the expected shape is a handful of
// calls during process start-up.
func Register[T any](name string, extract func(context.Context) (T, bool), opts ...Option) {
	if name == "" {
		panic("limelight: Register called with an empty field name")
	}
	if extract == nil {
		panic("limelight: Register(" + name + ") called with a nil extractor")
	}
	f := &field{
		name:    name,
		attrKey: name,
		extract: func(ctx context.Context) (string, bool) {
			v, ok := extract(ctx)
			if !ok {
				return "", false
			}
			return stringify(any(v)), true
		},
	}
	for _, opt := range opts {
		opt(f)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = f
}

// As maps a registered field onto an OpenTelemetry semantic-convention
// attribute key (semconv 1.44.0), so downstream tools join on it for free.
//
//	limelight.Register("userID", user.IDFromContext, limelight.As("user.id"))
//
// Without an As the field is emitted under its registered name.
func As(attrKey string) Option {
	return func(f *field) { f.attrKey = attrKey }
}

// Registered returns the registered field names in sorted order. It exists for
// diagnostics — the control endpoint reports it when a request targets a field
// nobody registered.
func Registered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func lookup(name string) (*field, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[name]
	return f, ok
}

// stringify keeps the emitted value a string regardless of the extractor's type
// parameter. Targeting compares strings, so extraction and matching can never
// disagree about what a value "is".
func stringify(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	case error:
		return s.Error()
	}
	return fmt.Sprint(v)
}
