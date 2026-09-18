package toolexec_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// program is the fixture: one tagged method, called twice, the second time
// panicking so the test can check where the panic is reported.
const program = `package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	limelight "github.com/stuparm/limelight/limelight-go"
)

type projectIDKey struct{}

func ProjectIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(projectIDKey{}).(string)
	return v, ok
}

type S struct{}

//limelight:method fields:"projectID"
func (s *S) M(ctx context.Context, boom bool) {
	if boom {
		panic("boom")
	}
}

func main() {
	limelight.Register("projectID", ProjectIDFromContext, limelight.As("project.id"))
	limelight.SetEmitter(limelight.NewLogEmitter(limelight.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil)))))
	if _, err := limelight.Enable(limelight.Config{
		TTL:   time.Minute,
		Match: map[string]string{"projectID": "abc-123"},
	}); err != nil {
		panic(err)
	}
	ctx := context.WithValue(context.Background(), projectIDKey{}, "abc-123")
	(&S{}).M(ctx, false)
	(&S{}).M(ctx, true)
}
`

// panicLine is where `panic("boom")` sits in program, 1-indexed. The whole point
// of splicing bytes instead of reprinting the AST is that this number survives
// instrumentation.
func panicLine(t *testing.T) int {
	t.Helper()
	for i, line := range strings.Split(program, "\n") {
		if strings.Contains(line, `panic("boom")`) {
			return i + 1
		}
	}
	t.Fatal("fixture no longer contains the panic")
	return 0
}

// setup writes the fixture module and builds the shim.
func setup(t *testing.T) (dir, shim string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds two binaries")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain")
	}

	runtimeDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	dir = t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", program)
	write("go.mod", fmt.Sprintf(`module limelightfixture

go 1.23

require github.com/stuparm/limelight/limelight-go v0.0.0

replace github.com/stuparm/limelight/limelight-go => %s
`, runtimeDir))

	shim = filepath.Join(t.TempDir(), "limelight")
	build := exec.Command("go", "build", "-o", shim, "github.com/stuparm/limelight/limelight-go/cmd/limelight")
	build.Dir = runtimeDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the shim: %v\n%s", err, out)
	}
	return dir, shim
}

// run executes the fixture, optionally through the shim, and returns its output.
func run(t *testing.T, dir string, toolexec string) (stdout, stderr string) {
	t.Helper()
	args := []string{"run"}
	if toolexec != "" {
		args = append(args, "-toolexec="+toolexec+" toolexec")
	}
	args = append(args, ".")

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	// The fixture lives outside the repo's workspace; GOWORK=off keeps go.work
	// from overriding its replace directive.
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run() // the fixture always panics; the exit code is expected to be non-zero
	return out.String(), errb.String()
}

func TestToolexecInstrumentsTheTag(t *testing.T) {
	dir, shim := setup(t)
	stdout, stderr := run(t, dir, shim)

	events := strings.Count(stdout, `"msg":"limelight"`)
	if events != 2 {
		t.Errorf("got %d events, want 2\nstdout:\n%s\nstderr:\n%s", events, stdout, stderr)
	}
	for _, want := range []string{`"method":"main.S.M"`, `"project.id":"abc-123"`, `"version":0`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %s:\n%s", want, stdout)
		}
	}
}

// The injected call goes on a line that already exists, and the rewritten copy
// carries a //line directive back to the original. A panic must therefore name
// the user's file at the user's line, not a temp path.
func TestInstrumentedPanicKeepsItsPosition(t *testing.T) {
	dir, shim := setup(t)
	_, stderr := run(t, dir, shim)

	want := fmt.Sprintf("main.go:%d", panicLine(t))
	if !strings.Contains(stderr, want) {
		t.Errorf("panic does not report %s:\n%s", want, stderr)
	}
	if strings.Contains(stderr, "limelight-") && strings.Contains(stderr, os.TempDir()) {
		t.Errorf("panic leaks the rewriter's temp directory:\n%s", stderr)
	}
}

// Without the shim the directive is an ordinary comment: the program runs and
// emits nothing. This is the property the vet analyzer exists to make visible.
func TestWithoutTheShimTheTagIsInert(t *testing.T) {
	dir, _ := setup(t)
	stdout, stderr := run(t, dir, "")

	if strings.Contains(stdout, `"msg":"limelight"`) {
		t.Errorf("emitted without the shim:\n%s", stdout)
	}
	want := fmt.Sprintf("main.go:%d", panicLine(t))
	if !strings.Contains(stderr, want) {
		t.Errorf("uninstrumented panic does not report %s:\n%s", want, stderr)
	}
}
