package limelight

import "context"

// Option configures a registered field.
type Option func(*field)

type field struct {
	name    string
	attrKey string
}

// Register binds a field name — the name used in a //limelight:method
// fields:"..." directive — to a function that pulls it out of a context.
//
// The tag never names a raw context key: idiomatic keys are unexported types
// owned by another package, and generated code cannot reference them. The
// registry is what makes limelight portable across codebases.
//
//	limelight.Register("projectID", project.IDFromContext, limelight.As("project.id"))
//
// TODO(v0): unimplemented.
func Register[T any](name string, extract func(context.Context) (T, bool), opts ...Option) {
	panic("limelight: Register not implemented")
}

// As maps a registered field onto an OpenTelemetry semantic-convention
// attribute key (semconv 1.44.0), so downstream tools join on it for free.
//
// TODO(v0): unimplemented.
func As(attrKey string) Option {
	panic("limelight: As not implemented")
}
