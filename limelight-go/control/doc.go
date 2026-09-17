// Package control serves the enable/disable endpoint as a plain http.Handler —
// no web framework, so it mounts anywhere.
//
// The wire format is spec/enable-protocol.md, which is shared with every other
// SDK. Server-side: authn, rate limit, cap the TTL, audit who enabled what.
//
// v0 scope is per-pod: a POST behind a load balancer enables exactly one
// replica, and the response says which. A shared store comes later.
package control
