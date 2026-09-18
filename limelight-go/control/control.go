// Package control serves the enable/disable endpoint as a plain http.Handler —
// no web framework, so it mounts anywhere.
//
// The wire format is spec/enable-protocol.md, which is shared with every other
// SDK.
//
// This handler is deliberately not safe to expose as-is. It turns on emission
// of user identifiers and can flood a logging bill, so the caller wraps it in
// authentication, a rate limit and an audit log. What it does enforce is the
// TTL cap and the explicit-targeting rule, because those are the two mistakes
// that hurt even an authenticated operator.
//
// v0 scope is per-pod: a POST behind a load balancer enables exactly one
// replica, and the response says which.
package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	limelight "github.com/stuparm/limelight/limelight-go"
)

// ProtocolVersion is the enable-protocol version this handler speaks. A payload
// carrying anything else is rejected rather than guessed at.
const ProtocolVersion = 0

// MaxTTL is the server-side cap. The spec suggests an hour; a caller asking for
// more gets an error, not a silent truncation, so nobody believes they armed a
// longer window than they did.
const MaxTTL = time.Hour

// Prefix is where the routes live.
const Prefix = "/debug/limelight/"

type enableRequest struct {
	Version *int              `json:"version"`
	TTL     string            `json:"ttl"`
	Match   map[string]string `json:"match"`
	Methods []string          `json:"methods"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Handler returns the control routes. Mount it on the prefix it expects:
//
//	mux.Handle(control.Prefix, control.Handler())
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+Prefix+"enable", enable)
	mux.HandleFunc("POST "+Prefix+"disable", disable)
	mux.HandleFunc("GET "+Prefix+"status", status)
	return mux
}

func enable(w http.ResponseWriter, r *http.Request) {
	var req enableRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: %v", err)
		return
	}

	if req.Version == nil {
		writeError(w, http.StatusBadRequest, "missing %q; this handler speaks version %d", "version", ProtocolVersion)
		return
	}
	if *req.Version != ProtocolVersion {
		writeError(w, http.StatusBadRequest, "unsupported protocol version %d; this handler speaks %d", *req.Version, ProtocolVersion)
		return
	}
	if len(req.Methods) > 0 {
		// Honest 501 over silent acceptance: a caller who scoped a firehose to
		// one method glob and got everything would be worse off than one who
		// got an error.
		writeError(w, http.StatusNotImplemented, "%q filtering is not implemented in v0; omit it to target every tagged method", "methods")
		return
	}

	ttl, err := time.ParseDuration(req.TTL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%q must be a duration string such as \"10m\": %v", "ttl", err)
		return
	}
	if ttl > MaxTTL {
		writeError(w, http.StatusBadRequest, "%q of %s exceeds the server cap of %s", "ttl", ttl, MaxTTL)
		return
	}

	state, err := limelight.Enable(limelight.Config{TTL: ttl, Match: req.Match})
	if err != nil {
		var unregistered *limelight.UnregisteredFieldError
		switch {
		case errors.Is(err, limelight.ErrEmptyMatch),
			errors.Is(err, limelight.ErrNonPositiveTTL),
			errors.As(err, &unregistered):
			writeError(w, http.StatusBadRequest, "%v", err)
		default:
			writeError(w, http.StatusInternalServerError, "%v", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func disable(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, limelight.Disable())
}

func status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, limelight.Current())
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, code int, format string, args ...any) {
	writeJSON(w, code, errorResponse{Error: fmt.Sprintf(format, args...)})
}
