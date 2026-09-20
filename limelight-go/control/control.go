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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	limelight "github.com/stuparm/limelight/limelight-go"
)

// ProtocolVersion is the enable-protocol version this handler speaks. A payload
// carrying anything else is rejected rather than guessed at.
const ProtocolVersion = 0

// MaxTTL is the server-side cap on a targeted activation. The spec suggests an
// hour; a caller asking for more gets an error, not a silent truncation, so
// nobody believes they armed a longer window than they did.
const MaxTTL = time.Hour

// MaxMatchAllTTL is the cap when match_all is set. A firehose emits user
// identifiers for every request the process serves, so it is bounded far more
// tightly than a targeted activation — long enough to answer "is anything
// coming through at all", not long enough to fill a log bill.
const MaxMatchAllTTL = time.Minute

// maxBodyBytes bounds the request body. It is read whole because the protocol
// version has to be inspected before the rest is decoded.
const maxBodyBytes = 1 << 16

// Prefix is where the routes live.
const Prefix = "/debug/limelight/"

type enableRequest struct {
	Version  *int              `json:"version"`
	TTL      string            `json:"ttl"`
	Match    map[string]string `json:"match"`
	MatchAll bool              `json:"match_all"`
	Methods  []string          `json:"methods"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Mux is the part of *http.ServeMux that Mount needs. Any router with the same
// Handle signature satisfies it.
type Mux interface {
	Handle(pattern string, handler http.Handler)
}

// Mount registers the control routes on mux. It is the usual way in:
//
//	mux := http.NewServeMux()
//	control.Mount(mux)
//
// Deliberately not an init() that registers on http.DefaultServeMux, the way
// net/http/pprof does. This endpoint turns on emission of user identifiers, so
// arming it must be something a service did on purpose — not something a
// transitive import did to it. It would also be unreliable: most services never
// serve DefaultServeMux, so the blank import would quietly do nothing.
func Mount(mux Mux) {
	mux.Handle(Prefix, Handler())
}

// Wrap returns a handler that serves the control routes and passes everything
// else to next. Use it when the router is not a *http.ServeMux — gin, echo, chi
// and gorilla all satisfy http.Handler even where they do not satisfy Mux:
//
//	srv := &http.Server{Handler: control.Wrap(myRouter)}
func Wrap(next http.Handler) http.Handler {
	control := Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, Prefix) {
			control.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler returns the control routes as a plain http.Handler, for a service
// that wants to mount them itself — behind its own auth middleware, on an
// admin-only listener, or under a different prefix.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+Prefix+"enable", enable)
	mux.HandleFunc("POST "+Prefix+"disable", disable)
	mux.HandleFunc("GET "+Prefix+"status", status)
	return mux
}

func enable(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "reading body: %v", err)
		return
	}

	// The protocol version is read before anything else, and deliberately with
	// a lenient decoder. A client speaking a later version sends fields this
	// build has never heard of; decoding strictly first would answer it with a
	// complaint about an unknown field and never mention versions — which is
	// useless to the one caller the version field exists to serve.
	var probe struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: %v", err)
		return
	}
	if probe.Version == nil {
		writeError(w, http.StatusBadRequest, "missing %q; this handler speaks version %d", "version", ProtocolVersion)
		return
	}
	if *probe.Version != ProtocolVersion {
		writeError(w, http.StatusBadRequest, "unsupported protocol version %d; this handler speaks %d", *probe.Version, ProtocolVersion)
		return
	}

	var req enableRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed body: %v", err)
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
	maxTTL := MaxTTL
	if req.MatchAll {
		maxTTL = MaxMatchAllTTL
	}
	if ttl > maxTTL {
		if req.MatchAll {
			writeError(w, http.StatusBadRequest, "%q of %s exceeds the %s cap of %s", "ttl", ttl, "match_all", maxTTL)
			return
		}
		writeError(w, http.StatusBadRequest, "%q of %s exceeds the server cap of %s", "ttl", ttl, maxTTL)
		return
	}

	state, err := limelight.Enable(limelight.Config{TTL: ttl, Match: req.Match, MatchAll: req.MatchAll})
	if err != nil {
		var unregistered *limelight.UnregisteredFieldError
		switch {
		case errors.Is(err, limelight.ErrEmptyMatch),
			errors.Is(err, limelight.ErrMatchAndMatchAll),
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
