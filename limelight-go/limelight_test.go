package limelight

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// collector is an Emitter that remembers what it was handed.
type collector struct {
	mu     sync.Mutex
	events []Event
}

func (c *collector) Emit(_ context.Context, ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func (c *collector) drain() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.events
	c.events = nil
	return out
}

// reset clears the package-level registry and switch between tests.
func reset(t *testing.T) *collector {
	t.Helper()
	registryMu.Lock()
	registry = map[string]*field{}
	registryMu.Unlock()
	Disable()
	c := &collector{}
	SetEmitter(c)
	t.Cleanup(func() {
		Disable()
		registryMu.Lock()
		registry = map[string]*field{}
		registryMu.Unlock()
	})
	return c
}

type projectKey struct{}
type userKey struct{}

func projectID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(projectKey{}).(string)
	return v, ok
}

func userID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userKey{}).(string)
	return v, ok
}

func ctxWith(project, user string) context.Context {
	ctx := context.WithValue(context.Background(), projectKey{}, project)
	return context.WithValue(ctx, userKey{}, user)
}

func registerBoth() {
	Register("projectID", projectID, As("project.id"))
	Register("userID", userID, As("user.id"))
}

func TestEmitIsSilentWhileOff(t *testing.T) {
	c := reset(t)
	registerBoth()
	Emit(ctxWith("abc-123", "u-42"), "p.S.M", "projectID")
	if got := c.drain(); len(got) != 0 {
		t.Fatalf("emitted %d event(s) with the switch off: %+v", len(got), got)
	}
}

func TestEmitOnlyForTheTargetedIdentity(t *testing.T) {
	c := reset(t)
	registerBoth()
	if _, err := Enable(Config{TTL: time.Minute, Match: map[string]string{"projectID": "abc-123"}}); err != nil {
		t.Fatal(err)
	}

	Emit(ctxWith("zzz-999", "u-7"), "p.S.M", "projectID", "userID")
	if got := c.drain(); len(got) != 0 {
		t.Fatalf("a non-matching identity emitted: %+v", got)
	}

	Emit(ctxWith("abc-123", "u-42"), "p.S.M", "projectID", "userID")
	got := c.drain()
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Method != "p.S.M" {
		t.Errorf("Method = %q, want %q", got[0].Method, "p.S.M")
	}
	if got[0].Version != EventVersion {
		t.Errorf("Version = %d, want %d", got[0].Version, EventVersion)
	}
	// As() is what makes the downstream join work; the registered name must not
	// leak into the output.
	want := map[string]string{"project.id": "abc-123", "user.id": "u-42"}
	if len(got[0].Fields) != len(want) {
		t.Fatalf("Fields = %v, want %v", got[0].Fields, want)
	}
	for k, v := range want {
		if got[0].Fields[k] != v {
			t.Errorf("Fields[%q] = %q, want %q", k, got[0].Fields[k], v)
		}
	}
}

func TestAbsentFieldIsOmittedNotEmpty(t *testing.T) {
	c := reset(t)
	registerBoth()
	if _, err := Enable(Config{TTL: time.Minute, Match: map[string]string{"projectID": "abc-123"}}); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), projectKey{}, "abc-123") // no user
	Emit(ctx, "p.S.M", "projectID", "userID")
	got := c.drain()
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if _, present := got[0].Fields["user.id"]; present {
		t.Errorf("absent field emitted as %q; want it omitted", got[0].Fields["user.id"])
	}
}

func TestSwitchTurnsItselfOff(t *testing.T) {
	c := reset(t)
	registerBoth()
	if _, err := Enable(Config{TTL: 50 * time.Millisecond, Match: map[string]string{"projectID": "abc-123"}}); err != nil {
		t.Fatal(err)
	}
	ctx := ctxWith("abc-123", "u-42")
	Emit(ctx, "p.S.M", "projectID")
	if len(c.drain()) != 1 {
		t.Fatal("no event while the TTL was live")
	}

	deadline := time.Now().Add(2 * time.Second)
	for Current().Enabled {
		if time.Now().After(deadline) {
			t.Fatal("still enabled well past the TTL")
		}
		time.Sleep(5 * time.Millisecond)
	}
	Emit(ctx, "p.S.M", "projectID")
	if got := c.drain(); len(got) != 0 {
		t.Fatalf("emitted after expiry: %+v", got)
	}
}

// A second Enable extends the window. The first activation's timer must not
// take the second one down with it when it fires.
func TestReEnableSurvivesTheEarlierExpiry(t *testing.T) {
	c := reset(t)
	registerBoth()
	match := map[string]string{"projectID": "abc-123"}
	if _, err := Enable(Config{TTL: 40 * time.Millisecond, Match: match}); err != nil {
		t.Fatal(err)
	}
	if _, err := Enable(Config{TTL: 5 * time.Second, Match: match}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // well past the first TTL

	if !Current().Enabled {
		t.Fatal("the first activation's timer disabled the second one")
	}
	Emit(ctxWith("abc-123", "u-42"), "p.S.M", "projectID")
	if len(c.drain()) != 1 {
		t.Fatal("no event after re-enabling")
	}
}

func TestEnableRejectsUnboundedOrUntargeted(t *testing.T) {
	reset(t)
	registerBoth()
	match := map[string]string{"projectID": "abc-123"}

	if _, err := Enable(Config{TTL: 0, Match: match}); !errors.Is(err, ErrNonPositiveTTL) {
		t.Errorf("zero TTL: err = %v, want ErrNonPositiveTTL", err)
	}
	if _, err := Enable(Config{TTL: time.Minute}); !errors.Is(err, ErrEmptyMatch) {
		t.Errorf("empty match: err = %v, want ErrEmptyMatch", err)
	}

	var unregistered *UnregisteredFieldError
	_, err := Enable(Config{TTL: time.Minute, Match: map[string]string{"nope": "x"}})
	if !errors.As(err, &unregistered) {
		t.Fatalf("unknown field: err = %v, want *UnregisteredFieldError", err)
	}
	if unregistered.Field != "nope" {
		t.Errorf("Field = %q, want %q", unregistered.Field, "nope")
	}
	if Current().Enabled {
		t.Error("a rejected Enable turned the switch on anyway")
	}
}

func TestEveryMatchFieldMustAgree(t *testing.T) {
	c := reset(t)
	registerBoth()
	if _, err := Enable(Config{TTL: time.Minute, Match: map[string]string{
		"projectID": "abc-123",
		"userID":    "u-42",
	}}); err != nil {
		t.Fatal(err)
	}
	Emit(ctxWith("abc-123", "someone-else"), "p.S.M", "projectID")
	if got := c.drain(); len(got) != 0 {
		t.Fatalf("emitted when only one of two match fields agreed: %+v", got)
	}
	Emit(ctxWith("abc-123", "u-42"), "p.S.M", "projectID")
	if len(c.drain()) != 1 {
		t.Fatal("no event when both match fields agreed")
	}
}

func TestNonStringExtractorIsStringified(t *testing.T) {
	c := reset(t)
	type tenantKey struct{}
	Register("tenant", func(ctx context.Context) (int, bool) {
		v, ok := ctx.Value(tenantKey{}).(int)
		return v, ok
	}, As("tenant.id"))

	if _, err := Enable(Config{TTL: time.Minute, Match: map[string]string{"tenant": "7"}}); err != nil {
		t.Fatal(err)
	}
	Emit(context.WithValue(context.Background(), tenantKey{}, 7), "p.S.M", "tenant")
	got := c.drain()
	if len(got) != 1 || got[0].Fields["tenant.id"] != "7" {
		t.Fatalf("got %+v, want one event with tenant.id=7", got)
	}
}

// benchSetup puts the package in the state every tagged call in a real process
// sees: fields registered, switch off.
func benchSetup() context.Context {
	registryMu.Lock()
	registry = map[string]*field{}
	registryMu.Unlock()
	registerBoth()
	Disable()
	return ctxWith("abc-123", "u-42")
}
