package rewrite

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// parses is the load-bearing check: the rewriter emits text, not an AST, so the
// only proof that a splice is legal Go is re-parsing the result.
func parses(t *testing.T, src []byte) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "out.go", src, parser.SkipObjectResolution); err != nil {
		t.Fatalf("rewritten source does not parse: %v\n---\n%s", err, src)
	}
}

func lines(b []byte) int { return strings.Count(string(b), "\n") }

const withImport = `package main

import (
	"context"

	limelight "github.com/stuparm/limelight/limelight-go"
)

type Service struct{}

//limelight:method fields:"projectID,userID"
func (s *Service) CreateThing(ctx context.Context, name string) error {
	_ = limelight.Registered
	return nil
}
`

func TestInjectsGateAndKeepsLineNumbers(t *testing.T) {
	res, err := File("main.go", []byte(withImport))
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 {
		t.Fatalf("Count = %d, want 1", res.Count)
	}
	got := string(res.Source)
	want := `if limelight.On() { limelight.Emit(ctx, "main.Service.CreateThing", "projectID", "userID") };`
	if !strings.Contains(got, want) {
		t.Errorf("missing gate call %q in:\n%s", want, got)
	}
	if !strings.Contains(got, "CreateThing(ctx context.Context, name string) error {"+want) {
		t.Errorf("gate call is not the first statement, or moved to its own line:\n%s", got)
	}
	if strings.Contains(got, "limelight__gen") {
		t.Error("injected a second import even though the file already imports the runtime")
	}
	if n, orig := lines(res.Source), lines([]byte(withImport)); n != orig {
		t.Errorf("line count changed: %d -> %d; positions no longer map to the original", orig, n)
	}
	parses(t, res.Source)
}

const withoutImport = `package main

import "context"

type Service struct{}

//limelight:method fields:"projectID"
func (s *Service) CreateThing(ctx context.Context) error { return nil }

//limelight:method fields:"projectID"
func Plain(ctx context.Context) {}
`

func TestInjectsImportOnPackageClauseLine(t *testing.T) {
	res, err := File("main.go", []byte(withoutImport))
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 2 {
		t.Fatalf("Count = %d, want 2", res.Count)
	}
	got := string(res.Source)
	if !strings.HasPrefix(got, `package main; import limelight__gen "github.com/stuparm/limelight/limelight-go"`) {
		t.Errorf("import not spliced onto the package clause line:\n%s", firstLine(got))
	}
	if !strings.Contains(got, `if limelight__gen.On() { limelight__gen.Emit(ctx, "main.Service.CreateThing", "projectID") };`) {
		t.Errorf("method gate missing:\n%s", got)
	}
	if !strings.Contains(got, `if limelight__gen.On() { limelight__gen.Emit(ctx, "main.Plain", "projectID") };`) {
		t.Errorf("plain-function gate missing or mislabelled:\n%s", got)
	}
	if n, orig := lines(res.Source), lines([]byte(withoutImport)); n != orig {
		t.Errorf("line count changed: %d -> %d", orig, n)
	}
	parses(t, res.Source)
}

func TestWarnsInsteadOfInstrumenting(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "no context parameter",
			src: `package p
//limelight:method fields:"projectID"
func F(name string) {}
`,
			want: "takes no context.Context",
		},
		{
			name: "context is discarded",
			src: `package p
import "context"
//limelight:method fields:"projectID"
func F(_ context.Context) {}
`,
			want: "takes no context.Context",
		},
		{
			name: "directive not on a function",
			src: `package p
//limelight:method fields:"projectID"
type T struct{}
`,
			want: "not attached to a function declaration",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := File("p.go", []byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			if res.Count != 0 {
				t.Errorf("Count = %d, want 0", res.Count)
			}
			if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], tc.want) {
				t.Errorf("Warnings = %v, want one containing %q", res.Warnings, tc.want)
			}
		})
	}
}

func TestMalformedDirectiveIsAnError(t *testing.T) {
	for _, src := range []string{
		"package p\n//limelight:method\nfunc F() {}\n",
		"package p\n//limelight:method fields:projectID\nfunc F() {}\n",
		`package p
//limelight:method fields:"projectID,projectID"
func F() {}
`,
		`package p
//limelight:method scope:"all"
func F() {}
`,
	} {
		if _, err := File("p.go", []byte(src)); err == nil {
			t.Errorf("no error for malformed directive:\n%s", src)
		}
	}
}

func TestUntaggedFileIsLeftAlone(t *testing.T) {
	res, err := File("p.go", []byte("package p\n\nfunc F() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 || res.Source != nil {
		t.Errorf("Count = %d, Source = %q, want 0 and nil", res.Count, res.Source)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
