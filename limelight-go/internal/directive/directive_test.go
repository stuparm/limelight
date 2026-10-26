package directive

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseAccepts(t *testing.T) {
	for _, tc := range []struct {
		line string
		want []string
	}{
		{`//limelight:method fields:"projectID"`, []string{"projectID"}},
		{`//limelight:method fields:"projectID,userID"`, []string{"projectID", "userID"}},
		{`//limelight:method fields:"projectID, userID"`, []string{"projectID", "userID"}},
		{"//limelight:method\tfields:\"projectID\"", []string{"projectID"}},
		{`//limelight:method   fields:"projectID"   `, []string{"projectID"}},
	} {
		m, ok, err := Parse(tc.line)
		if !ok || err != nil {
			t.Errorf("Parse(%q) = ok %v, err %v; want a directive", tc.line, ok, err)
			continue
		}
		if !reflect.DeepEqual(m.Fields, tc.want) {
			t.Errorf("Parse(%q).Fields = %q, want %q", tc.line, m.Fields, tc.want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, tc := range []struct {
		line, want string
	}{
		{`//limelight:method`, "missing fields"},
		{`//limelight:method scope:"all"`, "unknown argument"},
		{`//limelight:method fields:projectID`, "not a double-quoted string"},
		{`//limelight:method fields:"projectID`, "not a quoted string"},
		{`//limelight:method fields:"projectID,projectID"`, "listed twice"},
		{`//limelight:method fields:"projectID,"`, "empty field name"},
		{`//limelight:method fields: "projectID"`, "no space allowed after fields:"},
		{`//limelight:method fields :"projectID"`, "no space allowed before the colon"},
		{`//limelight:method fields:"projectID" // emits project`, `trailing text "// emits project"`},
		// strconv.Unquote takes 'a' as a rune literal and `a` as a raw string.
		{`//limelight:method fields:'p'`, "not a double-quoted string"},
		{"//limelight:method fields:`projectID`", "not a double-quoted string"},
	} {
		_, ok, err := Parse(tc.line)
		if !ok {
			t.Errorf("Parse(%q) says not a directive; want a malformed one", tc.line)
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Parse(%q) err = %v, want one containing %q", tc.line, err, tc.want)
		}
	}
}

func TestSuspectFlags(t *testing.T) {
	for _, tc := range []struct {
		text, want string
	}{
		// Spacing: each of these is prose to the compiler and to the shim.
		{`// limelight:method fields:"projectID"`, "no space allowed between // and limelight"},
		{"//\tlimelight:method fields:\"projectID\"", "no space allowed between // and limelight"},
		{`//limelight: method fields:"projectID"`, "no space allowed around the colon"},
		{`//limelight :method fields:"projectID"`, "no space allowed around the colon"},
		{`// limelight:method`, "no space allowed between // and limelight"},
		{`//limelight:methodfields:"projectID"`, "missing space after //limelight:method"},
		{"//limelight:method" + string(rune(0xa0)) + `fields:"projectID"`, "followed by a space, not '\\u00a0' (U+00A0)"},
		{`//limelight:method:fields:"projectID"`, `followed by a space, not ':'`},

		// Case.
		{`//limelight:Method fields:"projectID"`, "lower-case"},
		{`//Limelight:method fields:"projectID"`, "lower-case"},

		// What gofmt saves the mistakes above as: it puts a space after the //
		// of any doc-comment line that is not directive-shaped. Every deviation
		// is named, or fixing the first leaves a line gofmt turns back to prose.
		{`// Limelight:method fields:"projectID"`, "no space allowed between // and limelight; directive names are lower-case"},
		{`// limelight: method fields:"projectID"`, "no space allowed between // and limelight; no space allowed around the colon"},
		{`// lime-light:method fields:"projectID"`, "lime-light:method is misspelled"},
		{`// limelight:metod fields:"projectID"`, "limelight:metod is misspelled"},
		{`// limelight:method fields="projectID"`, "no space allowed between // and limelight"},

		// Typos in the verb.
		{`//limelight:metod fields:"projectID"`, "did you mean //limelight:method"},
		{`//limelight:mehtod fields:"projectID"`, "did you mean //limelight:method"},
		{`//limelight:methods fields:"projectID"`, "did you mean //limelight:method"},
		{`//limelight:meth`, "did you mean //limelight:method"},
		{`//limelight:`, "names no directive"},
		{`//limelight:frobnicate`, "unknown directive //limelight:frobnicate"},

		// Typos in the namespace — the shim's byte prefilter never sees these.
		{`//limelght:method fields:"projectID"`, "did you mean //limelight:method"},
		{`//lime-light:method fields:"projectID"`, "did you mean //limelight:method"},
		{`//limeligth:metod fields:"projectID"`, "did you mean //limelight:method"},

		// The tag ladder: designed, not built.
		{`//limelight:type`, "//limelight:type is not implemented"},
		{`//limelight:package fields:"projectID"`, "//limelight:package is not implemented"},
		{`//limelight:skip`, "//limelight:skip is not implemented"},
		{`// limelight:type`, "//limelight:type is not implemented"},

		// Block comments.
		{`/* limelight:method fields:"projectID" */`, "must be a // line comment"},
		{`/*limelight:method fields:"projectID"*/`, "must be a // line comment"},
		{"/*\n//limelight:method fields:\"projectID\"\n*/", "must be a // line comment"},
		{"/*\n * limelight:method fields:\"projectID\"\n */", "must be a // line comment"},
	} {
		msg, ok := Suspect(tc.text)
		if !ok {
			t.Errorf("Suspect(%q) = not suspect; want %q", tc.text, tc.want)
			continue
		}
		if !strings.Contains(msg, tc.want) {
			t.Errorf("Suspect(%q) = %q, want it to contain %q", tc.text, msg, tc.want)
		}
	}
}

// Prose about limelight has the same shape as a mistyped directive. A false
// positive here fails somebody's CI over a comment, so every one of these must
// stay prose.
func TestSuspectLeavesProseAlone(t *testing.T) {
	for _, text := range []string{
		// The forms a doc comment about limelight actually takes.
		`// limelight: method tagging is entry-only`,
		`// Limelight: methods are tagged at compile time`,
		`// limelight: skip this handler, it has no identity`,
		`// limelight: type ladder is not built yet`,
		`// limelight: method fields are listed in the spec`,
		`// limelight is off by default`,
		`// limelight`,
		`/* limelight: method tagging is entry-only */`,

		// Mentions of the directive anywhere but the start of the comment.
		`// A plain go test compiles //limelight:method as an ordinary comment, so the`,
		"//\t//limelight:method fields:\"projectID\"",
		`// Write //limelight:type and it is parsed as an ordinary comment`,
		`// want +1 "unknown directive //limelight:metod"`,

		// Other tools' directives.
		`//go:generate stringer -type=Kind`,
		`//go:noinline`,
		`//nolint:errcheck`,
		`//lint:ignore SA1019 reason`,
		`//line main.go:10`,
		`//export Foo`,

		// Valid directives — and malformed ones, which are Parse's to report.
		`//limelight:method fields:"projectID"`,
		`//limelight:method`,
		`//limelight:method fields: "projectID"`,

		// Not a comment.
		`limelight:method fields:"projectID"`,
	} {
		if msg, ok := Suspect(text); ok {
			t.Errorf("Suspect(%q) = %q; want it left alone", text, msg)
		}
	}
}

func TestDistance(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"method", "method", 0},
		{"metod", "method", 1},
		{"mehtod", "method", 1}, // a transposition is one edit
		{"methods", "method", 1},
		{"", "method", 6},
		{"lime-light", "limelight", 1},
		{"nolint", "limelight", 6},
	} {
		if got := distance(tc.a, tc.b); got != tc.want {
			t.Errorf("distance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
