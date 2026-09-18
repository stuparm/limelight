// Package directive parses //limelight:method comment directives into a typed
// form shared by the codemod and the analyzer.
package directive

import (
	"fmt"
	"strconv"
	"strings"
)

// Prefix is the directive every tagged method carries. A Go comment directive
// has no space after the // — that is what distinguishes it from prose.
const Prefix = "//limelight:method"

// Method is a parsed //limelight:method directive.
type Method struct {
	// Fields are the registered field names to emit, in directive order.
	Fields []string
}

// Parse reads one comment line. It reports ok=false for any line that is not a
// limelight directive, so callers can hand it every comment in a doc group.
//
// An error means the line *is* a limelight directive but is malformed — that is
// worth failing the build over, since the alternative is a tag that silently
// emits nothing.
func Parse(line string) (m Method, ok bool, err error) {
	line = strings.TrimRight(line, " \t")
	if line != Prefix && !strings.HasPrefix(line, Prefix+" ") && !strings.HasPrefix(line, Prefix+"\t") {
		return Method{}, false, nil
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, Prefix))
	if rest == "" {
		return Method{}, true, fmt.Errorf("%s: missing fields:\"...\" — a directive with no fields emits nothing", Prefix)
	}

	const key = "fields:"
	if !strings.HasPrefix(rest, key) {
		return Method{}, true, fmt.Errorf("%s: unknown argument %q, expected fields:\"...\"", Prefix, rest)
	}
	value := strings.TrimPrefix(rest, key)
	unquoted, uerr := strconv.Unquote(value)
	if uerr != nil {
		return Method{}, true, fmt.Errorf("%s: fields value %s is not a quoted string", Prefix, value)
	}

	seen := map[string]bool{}
	var fields []string
	for _, raw := range strings.Split(unquoted, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			return Method{}, true, fmt.Errorf("%s: empty field name in fields:%q", Prefix, unquoted)
		}
		if seen[name] {
			return Method{}, true, fmt.Errorf("%s: field %q listed twice", Prefix, name)
		}
		seen[name] = true
		fields = append(fields, name)
	}
	if len(fields) == 0 {
		return Method{}, true, fmt.Errorf("%s: fields:\"\" names nothing", Prefix)
	}
	return Method{Fields: fields}, true, nil
}
