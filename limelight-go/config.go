package limelight

// The switch. Holds the active targeting config behind an atomic pointer so the
// hot path is a single load, and lets a ticker goroutine nil it at expiry —
// never a time.Now() call per invocation.
//
// TODO(v0): atomic.Pointer[config], TTL expiry, identity match, Enabled gate.
