package control_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	limelight "github.com/stuparm/limelight/limelight-go"
	"github.com/stuparm/limelight/limelight-go/control"
)

type projectKey struct{}

func TestMain(m *testing.M) {
	limelight.Register("projectID", func(ctx context.Context) (string, bool) {
		v, ok := ctx.Value(projectKey{}).(string)
		return v, ok
	}, limelight.As("project.id"))
	m.Run()
}

func post(t *testing.T, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	control.Handler().ServeHTTP(rec, req)
	return rec
}

func TestEnableAndDisable(t *testing.T) {
	t.Cleanup(func() { limelight.Disable() })

	rec := post(t, control.Prefix+"enable", `{"version":0,"ttl":"10m","match":{"projectID":"abc-123"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable: status %d, body %s", rec.Code, rec.Body)
	}
	var state limelight.State
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Enabled {
		t.Error("enabled = false")
	}
	// v0 is honest about reaching one replica rather than implying cluster scope.
	if state.Scope != "pod" {
		t.Errorf("scope = %q, want %q", state.Scope, "pod")
	}
	if state.Instance == "" {
		t.Error("instance is empty; a caller behind a load balancer cannot tell which replica answered")
	}
	if state.ExpiresAt == nil {
		t.Error("expiresAt is missing; the caller cannot tell when it turns itself off")
	}
	if !limelight.Current().Enabled {
		t.Error("the switch is not actually on")
	}

	rec = post(t, control.Prefix+"disable", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: status %d, body %s", rec.Code, rec.Body)
	}
	if limelight.Current().Enabled {
		t.Error("still enabled after disable")
	}
}

func TestEnableRejects(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"missing version", `{"ttl":"10m","match":{"projectID":"abc-123"}}`, http.StatusBadRequest},
		{"unknown version", `{"version":9,"ttl":"10m","match":{"projectID":"abc-123"}}`, http.StatusBadRequest},
		{"empty match is a firehose", `{"version":0,"ttl":"10m"}`, http.StatusBadRequest},
		{"unregistered field", `{"version":0,"ttl":"10m","match":{"nope":"x"}}`, http.StatusBadRequest},
		{"ttl over the cap", `{"version":0,"ttl":"24h","match":{"projectID":"abc-123"}}`, http.StatusBadRequest},
		{"ttl not a duration", `{"version":0,"ttl":"soon","match":{"projectID":"abc-123"}}`, http.StatusBadRequest},
		{"malformed json", `{`, http.StatusBadRequest},
		{"unknown field", `{"version":0,"ttl":"10m","match":{"projectID":"a"},"who":"me"}`, http.StatusBadRequest},
		{"methods glob is not implemented", `{"version":0,"ttl":"10m","match":{"projectID":"a"},"methods":["vnet.*"]}`, http.StatusNotImplemented},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { limelight.Disable() })
			rec := post(t, control.Prefix+"enable", tc.body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body %s", rec.Code, tc.want, rec.Body)
			}
			if limelight.Current().Enabled {
				t.Error("a rejected request turned the switch on")
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error == "" {
				t.Errorf("no error message for the caller: %s", rec.Body)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	t.Cleanup(func() { limelight.Disable() })
	req := httptest.NewRequest(http.MethodGet, control.Prefix+"status", nil)
	rec := httptest.NewRecorder()
	control.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var state limelight.State
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Enabled {
		t.Error("enabled before anything enabled it")
	}
}

func TestEnableIsPostOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, control.Prefix+"enable", nil)
	rec := httptest.NewRecorder()
	control.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET enable: status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestMount(t *testing.T) {
	t.Cleanup(func() { limelight.Disable() })
	mux := http.NewServeMux()
	control.Mount(mux)

	req := httptest.NewRequest(http.MethodPost, control.Prefix+"enable",
		strings.NewReader(`{"version":0,"ttl":"10m","match":{"projectID":"abc-123"}}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	if !limelight.Current().Enabled {
		t.Error("Mount registered the routes but the switch did not flip")
	}
}

// Wrap has to serve the control routes and leave every other path to the app.
func TestWrapPassesEverythingElseThrough(t *testing.T) {
	t.Cleanup(func() { limelight.Disable() })
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("app"))
	})
	h := control.Wrap(app)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/things", nil))
	if got := rec.Body.String(); got != "app" {
		t.Errorf("app route returned %q, want %q", got, "app")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, control.Prefix+"status", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "app") {
		t.Errorf("control route fell through to the app: %d %s", rec.Code, rec.Body)
	}

	// A path that merely starts with "/debug" is the app's business, not ours.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debugging", nil))
	if got := rec.Body.String(); got != "app" {
		t.Errorf("/debugging returned %q, want %q", got, "app")
	}
}
