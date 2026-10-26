// Package directive parses //limelight:method comment directives into a typed
// form shared by the codemod and the analyzer.
package directive

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
		if name, _, found := strings.Cut(rest, ":"); found && strings.TrimSpace(name) == "fields" {
			return Method{}, true, fmt.Errorf("%s: no space allowed before the colon in fields:\"...\"", Prefix)
		}
		return Method{}, true, fmt.Errorf("%s: unknown argument %q, expected fields:\"...\"", Prefix, rest)
	}
	value := strings.TrimPrefix(rest, key)
	if strings.TrimLeft(value, " \t") != value {
		return Method{}, true, fmt.Errorf("%s: no space allowed after fields: — write fields:\"...\"", Prefix)
	}
	// Only a double-quoted string: strconv.Unquote would also take 'a' as a rune
	// literal and `a` as a raw string, and neither is what anyone meant.
	if !strings.HasPrefix(value, `"`) {
		return Method{}, true, fmt.Errorf("%s: fields value %s is not a double-quoted string", Prefix, value)
	}
	quoted, qerr := strconv.QuotedPrefix(value)
	if qerr != nil {
		return Method{}, true, fmt.Errorf("%s: fields value %s is not a quoted string", Prefix, value)
	}
	if trailing := strings.TrimSpace(value[len(quoted):]); trailing != "" {
		return Method{}, true, fmt.Errorf("%s: trailing text %q after fields:%s — the directive line holds nothing else", Prefix, trailing, quoted)
	}
	unquoted, _ := strconv.Unquote(quoted) // QuotedPrefix vouched for it

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

// namespace and verb are the two halves of Prefix, as Suspect compares them.
const (
	namespace = "limelight"
	verb      = "method"
)

// rungs are the tag-ladder directives the design names but nothing implements.
var rungs = map[string]bool{"type": true, "package": true, "skip": true}

// Suspect reports a comment that looks like an attempt at a limelight
// directive but that Parse does not recognise — so the compiler and the shim
// both read it as prose, and the tag it was meant to be silently does nothing.
// msg says what is wrong. text is the comment as the parser gives it, with its
// // or /* */ markers.
//
// A comment Parse recognises is never suspect — Parse owns everything wrong
// with it — so each comment gets at most one verdict. Suspect looks at the
// start of the comment and nowhere else, so prose that merely mentions
// //limelight:method is never flagged.
func Suspect(text string) (msg string, ok bool) {
	if _, isDirective, _ := Parse(text); isDirective {
		return "", false
	}
	if body, isBlock := strings.CutPrefix(text, "/*"); isBlock {
		return suspectBlock(strings.TrimSuffix(body, "*/"))
	}
	body, isLine := strings.CutPrefix(text, "//")
	if !isLine {
		return "", false
	}
	h, found := splitHead(body)
	if !found {
		return "", false
	}
	if h.bare() {
		return suspectBare(h)
	}
	return suspectSpaced(h)
}

// head is the start of a comment cut at the shape of a directive:
// <lead><word><sp1>:<sp2><verb><rest>.
type head struct {
	lead, word, sp1, sp2, verb, rest string
}

// bare reports the //word:verb shape, with no space anywhere before the verb.
// Go reserves that shape for directives, so it is never prose.
func (h head) bare() bool { return h.lead == "" && h.sp1 == "" && h.sp2 == "" }

// splitHead cuts body into a head. It reports false when body does not have
// the word-colon shape at all.
func splitHead(body string) (head, bool) {
	var h head
	s := body
	h.lead, s = cutFunc(s, unicode.IsSpace)
	h.word, s = cutFunc(s, isWordRune)
	h.sp1, s = cutFunc(s, unicode.IsSpace)
	if h.word == "" || !strings.HasPrefix(s, ":") {
		return head{}, false
	}
	h.sp2, s = cutFunc(s[1:], unicode.IsSpace)
	h.verb, h.rest = cutFunc(s, isLetter)
	return h, true
}

// suspectBare judges the //word:verb shape. With the namespace spelled exactly,
// anything Parse rejected is a mistake; with it misspelled, only a verb that is
// recognisably ours makes it one.
func suspectBare(h head) (string, bool) {
	lower := strings.ToLower(h.verb)
	if h.word != namespace {
		if !ours(h.word) || (distance(lower, verb) > 2 && !rungs[lower]) {
			return "", false
		}
		if rungs[lower] {
			return notImplemented(lower), true
		}
		if strings.ToLower(h.word) == namespace && lower == verb {
			return "directive names are lower-case — write " + Prefix, true
		}
		return fmt.Sprintf("//%s:%s is not a limelight directive — did you mean %s?", h.word, h.verb, Prefix), true
	}

	switch {
	case h.verb == verb:
		// Parse already accepted every //limelight:method followed by an ASCII
		// space, a tab or nothing, so the next rune is something else: a colon,
		// a digit, or a non-breaking space pasted from a web page.
		r, _ := utf8.DecodeRuneInString(h.rest)
		return fmt.Sprintf("%s must be followed by a space, not %q (%U)", Prefix, r, r), true
	case strings.HasPrefix(h.verb, verb) && strings.HasPrefix(h.verb[len(verb):], "fields"):
		return "missing space after " + Prefix, true
	case lower == verb:
		return "directive names are lower-case — write " + Prefix, true
	case rungs[lower]:
		return notImplemented(lower), true
	case h.verb == "":
		return fmt.Sprintf("//%s: names no directive — did you mean %s?", namespace, Prefix), true
	case distance(lower, verb) <= 2:
		return fmt.Sprintf("unknown directive //%s:%s — did you mean %s?", namespace, h.verb, Prefix), true
	}
	return fmt.Sprintf("unknown directive //%s:%s — only %s exists, so this comment does nothing", namespace, h.verb, Prefix), true
}

// suspectSpaced judges a head with a space somewhere before the verb. That
// shape is also how people write prose about limelight, so it takes a
// recognisable namespace, a recognisable verb, and nothing after the verb
// except what a directive would carry.
//
// It is also the shape most mistakes end up in: gofmt puts a space after the
// // of any doc-comment line that is not directive-shaped, so a tag written
// //Limelight:method or //lime-light:method is saved as // Limelight:method.
// That is why the message lists every deviation — fixing only the first would
// leave a line gofmt turns back into prose.
func suspectSpaced(h head) (string, bool) {
	lower := strings.ToLower(h.verb)
	if !ours(h.word) || !directiveTail(h.rest) {
		return "", false
	}
	if rungs[lower] {
		return notImplemented(lower), true
	}
	if distance(lower, verb) > 2 {
		return "", false
	}
	var reasons []string
	if h.lead != "" {
		reasons = append(reasons, "no space allowed between // and "+namespace)
	}
	if h.sp1 != "" || h.sp2 != "" {
		reasons = append(reasons, "no space allowed around the colon")
	}
	if strings.ToLower(h.word) != namespace || lower != verb {
		reasons = append(reasons, fmt.Sprintf("%s:%s is misspelled", h.word, h.verb))
	} else if h.word != namespace || h.verb != verb {
		reasons = append(reasons, "directive names are lower-case")
	}
	return fmt.Sprintf("%s, so this is an ordinary comment that does nothing — write %s", strings.Join(reasons, "; "), Prefix), true
}

// suspectBlock flags a directive written inside /* */. Go only reads
// directives from // line comments, and a block comment is where prose lives,
// so every line is held to the spaced shape's standard.
func suspectBlock(body string) (string, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "*"))
		line = strings.TrimPrefix(line, "//")
		h, found := splitHead(line)
		if !found || strings.ToLower(h.word) != namespace || !directiveTail(h.rest) {
			continue
		}
		if lower := strings.ToLower(h.verb); distance(lower, verb) <= 2 || rungs[lower] {
			return "a limelight directive must be a // line comment — inside /* */ it is an ordinary comment that does nothing", true
		}
	}
	return "", false
}

// directiveTail reports whether rest is what follows a verb in a directive:
// nothing, or its arguments — "fields" then a colon or an equals sign, not
// "fields are listed in the spec".
func directiveTail(rest string) bool {
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.HasPrefix(rest, `:"`) {
		return true
	}
	after, ok := strings.CutPrefix(rest, "fields")
	after = strings.TrimSpace(after)
	return ok && (strings.HasPrefix(after, ":") || strings.HasPrefix(after, "="))
}

// ours reports whether word is the namespace, give or take two typos.
func ours(word string) bool {
	return distance(strings.ToLower(word), namespace) <= 2
}

func notImplemented(rung string) string {
	return fmt.Sprintf("//%s:%s is not implemented — only %s exists, so this comment does nothing", namespace, rung, Prefix)
}

// cutFunc splits s after its longest prefix of runes satisfying f.
func cutFunc(s string, f func(rune) bool) (prefix, rest string) {
	i := strings.IndexFunc(s, func(r rune) bool { return !f(r) })
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

func isLetter(r rune) bool { return r < utf8.RuneSelf && unicode.IsLetter(r) }

// isWordRune admits - and _ so that //lime-light:method is one misspelled
// word, not a word and some punctuation.
func isWordRune(r rune) bool {
	return isLetter(r) || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

// distance is the optimal-string-alignment edit distance: insertions,
// deletions, substitutions and adjacent transpositions each cost one, so
// "mehtod" is one typo from "method", not two. The inputs are short ASCII
// words, so bytes are runes.
func distance(a, b string) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}
