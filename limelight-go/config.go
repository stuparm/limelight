package limelight

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Config is one activation of the switch: who to trace, and for how long.
type Config struct {
	// TTL bounds the activation. Required — there is no way to turn limelight
	// on forever, by design.
	TTL time.Duration

	// Match is a registered field name to the value it must equal for a call
	// to emit. Every entry must match. It is evaluated by the same registered
	// extractors that produce output, so targeting and emission can never
	// disagree.
	//
	// Required and non-empty: an empty Match means "every request on this
	// process", which is a firehose nobody asks for by accident.
	Match map[string]string
}

// State is what the switch reports about itself — the response body of the
// enable endpoint, and what Current returns.
type State struct {
	Enabled bool `json:"enabled"`
	// Scope is "pod": a POST behind a load balancer reaches exactly one
	// replica. v0 is honest about that rather than implying cluster scope.
	Scope     string     `json:"scope,omitempty"`
	Instance  string     `json:"instance,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// active is the immutable snapshot the hot path reads. Replaced wholesale, never
// mutated, so Emit needs one atomic load and no lock.
type active struct {
	match     map[string]string
	expiresAt time.Time
}

var (
	current atomic.Pointer[active]

	timerMu sync.Mutex
	timer   *time.Timer

	instanceOnce sync.Once
	instanceID   string
)

// ErrEmptyMatch is returned when Enable is called without targeting. See
// Config.Match.
var ErrEmptyMatch = errors.New("limelight: Config.Match must name at least one field — an empty match is a firehose")

// ErrNonPositiveTTL is returned when Enable is called without a bound.
var ErrNonPositiveTTL = errors.New("limelight: Config.TTL must be positive")

// UnregisteredFieldError reports a Match against a field nobody Register'd. It
// is a request error, not a server error: the caller named a field this binary
// does not know how to extract.
type UnregisteredFieldError struct {
	Field      string
	Registered []string
}

func (e *UnregisteredFieldError) Error() string {
	return fmt.Sprintf("limelight: match field %q is not registered (registered: %v)", e.Field, e.Registered)
}

// Enable turns emission on for the matching identity until the TTL expires,
// replacing any activation already in flight. It never blocks the hot path:
// the new config is published with a single atomic store.
func Enable(cfg Config) (State, error) {
	if cfg.TTL <= 0 {
		return State{}, ErrNonPositiveTTL
	}
	if len(cfg.Match) == 0 {
		return State{}, ErrEmptyMatch
	}
	match := make(map[string]string, len(cfg.Match))
	for name, want := range cfg.Match {
		if _, ok := lookup(name); !ok {
			return State{}, &UnregisteredFieldError{Field: name, Registered: Registered()}
		}
		match[name] = want
	}

	expiresAt := time.Now().Add(cfg.TTL)
	a := &active{match: match, expiresAt: expiresAt}
	current.Store(a)

	timerMu.Lock()
	defer timerMu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	// CompareAndSwap, not Store(nil): a later Enable that lands before this
	// timer fires must not be turned off by the older activation's expiry.
	timer = time.AfterFunc(cfg.TTL, func() { current.CompareAndSwap(a, nil) })

	return State{Enabled: true, Scope: "pod", Instance: Instance(), ExpiresAt: &expiresAt}, nil
}

// Disable turns emission off immediately.
func Disable() State {
	current.Store(nil)
	timerMu.Lock()
	defer timerMu.Unlock()
	if timer != nil {
		timer.Stop()
		timer = nil
	}
	return State{Enabled: false, Scope: "pod", Instance: Instance()}
}

// Current reports whether the switch is on, and until when.
func Current() State {
	a := current.Load()
	if a == nil {
		return State{Enabled: false, Scope: "pod", Instance: Instance()}
	}
	expiresAt := a.expiresAt
	return State{Enabled: true, Scope: "pod", Instance: Instance(), ExpiresAt: &expiresAt}
}

// Instance identifies this process in an enable response, so a user who POSTs
// through a load balancer can see which replica they actually hit.
func Instance() string {
	instanceOnce.Do(func() {
		host, err := os.Hostname()
		if err != nil || host == "" {
			host = "unknown"
		}
		instanceID = host + "-" + strconv.Itoa(os.Getpid())
	})
	return instanceID
}
