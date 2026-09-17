// Package emit turns extracted fields into output. The emitter is an interface
// with a template-generated implementation per backend (slog first, then otel,
// zap, or whatever a team writes for its house logger).
//
// Keeping this behind an interface is what leaves the baggage-vs-custom-emitter
// question (docs/design.md) open until there is evidence.
package emit
