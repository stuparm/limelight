package auto_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	limelight "github.com/stuparm/limelight/limelight-go"
	"github.com/stuparm/limelight/limelight-go/control"

	// The import under test. Its init is the entire feature.
	_ "github.com/stuparm/limelight/limelight-go/control/auto"
)

func TestBlankImportRegistersOnDefaultServeMux(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, control.Prefix+"status", nil)
	rec := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; the blank import did not register", rec.Code, http.StatusOK)
	}
	var state limelight.State
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if state.Scope != "pod" {
		t.Errorf("scope = %q, want %q", state.Scope, "pod")
	}
}

// Registering a subtree must not turn limelight into a catch-all for the
// process's default mux.
func TestItClaimsOnlyItsOwnPrefix(t *testing.T) {
	for _, path := range []string{"/", "/api/things", "/debugging"} {
		rec := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404 — limelight claimed a path that is not its own", path, rec.Code)
		}
	}
}
