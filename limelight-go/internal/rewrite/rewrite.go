// Package rewrite injects the limelight gate into tagged methods.
//
// It splices bytes at offsets rather than reprinting the AST. That is not a
// micro-optimisation: every inserted fragment goes on a line that already
// exists, so the rewritten file has exactly the same line count as the original
// and every position in a panic, a debugger or a coverage profile still points
// at the user's real source.
package rewrite

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/stuparm/limelight/limelight-go/internal/directive"
)

// RuntimePath is the module path of the runtime package the injected call
// targets, and runtimePkg is the name it binds to when imported without an
// explicit local name. They differ — the directory is limelight-go, the package
// is limelight — which is exactly the case a naive path-tail guess gets wrong.
const (
	RuntimePath = "github.com/stuparm/limelight/limelight-go"
	runtimePkg  = "limelight"
)

// genLocal is the import name used when a file does not already import the
// runtime. The double underscore keeps it clear of anything hand-written.
const genLocal = "limelight__gen"

// Result is the outcome of rewriting one file.
type Result struct {
	// Source is the rewritten file. It is nil when Count is 0.
	Source []byte
	// Count is the number of methods instrumented.
	Count int
	// Warnings are directives that could not be honoured. They do not fail the
	// build here — the analyzer is what turns these into build errors — but a
	// silently ignored tag is the failure mode limelight exists to avoid, so
	// the toolexec driver prints them.
	Warnings []string
}

type splice struct {
	offset int
	text   string
}

// MissingImport is what is wrong with a package that holds a tagged method but
// does not import the runtime, and how to fix it. The shim and the vet analyzer
// both say it, so the two cannot drift apart.
//
// It says "non-test file" because that is what go build sees: an import only a
// _test.go file carries satisfies go test and still fails go build.
const MissingImport = "does not import " + RuntimePath + ", and -toolexec cannot add to the build graph. " +
	"Add  import _ \"" + RuntimePath + "\"  to any non-test file of the package"

// Problem is a directive that cannot work as written.
type Problem struct {
	Pos token.Pos
	// Msg says what is wrong. It carries no position; the caller holds the
	// FileSet to format Pos with.
	Msg string
	// Err marks a malformed directive, which File refuses to build past. Every
	// other problem leaves its method uninstrumented and the build going.
	Err bool
}

// Tagged is a method the rewriter instruments.
type Tagged struct {
	Fn        *ast.FuncDecl
	Directive *ast.Comment
	Method    directive.Method
	// Ctx is the name of the context.Context parameter the gate reads.
	Ctx string
}

// Scan finds every tagged method in f, and every directive that cannot work as
// written.
//
// It is the one place that decides whether a tag takes effect: File
// instruments exactly the methods it returns, and the vet analyzer reports
// exactly the problems it returns, so the two cannot disagree about a tag.
func Scan(f *ast.File) (tagged []Tagged, problems []Problem) {
	ctxLocal := importLocalName(f, "context", "context")
	// judged holds the directives the declaration loop has ruled on, so the
	// sweep over every comment below does not rule on them twice.
	judged := map[*ast.Comment]bool{}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		var first *ast.Comment
		var m directive.Method
		malformed := false
		for _, c := range fn.Doc.List {
			parsed, isDirective, err := directive.Parse(c.Text)
			if !isDirective {
				continue
			}
			judged[c] = true
			if first != nil {
				// Not judged on its fields: only the first directive ever took
				// effect, and a field error in one that never did must not start
				// failing a build that passes today.
				problems = append(problems, Problem{Pos: c.Pos(), Msg: fmt.Sprintf("a second %s in this doc comment is ignored — list every field in the first", directive.Prefix)})
				continue
			}
			first = c
			if err != nil {
				problems = append(problems, Problem{Pos: c.Pos(), Msg: err.Error(), Err: true})
				malformed = true
				continue
			}
			m = parsed
		}
		if first == nil || malformed {
			continue
		}
		if fn.Body == nil {
			problems = append(problems, Problem{Pos: fn.Pos(), Msg: fmt.Sprintf("%s on %s, which has no body", directive.Prefix, fn.Name.Name)})
			continue
		}
		ctxName, ok := contextParam(fn, ctxLocal)
		if !ok {
			problems = append(problems, Problem{Pos: fn.Pos(), Msg: fmt.Sprintf("%s on %s, which takes no context.Context — nothing to extract from", directive.Prefix, fn.Name.Name)})
			continue
		}
		tagged = append(tagged, Tagged{Fn: fn, Directive: first, Method: m, Ctx: ctxName})
	}

	// A directive that is not a function's doc comment is inert, and so is one
	// that is almost a directive: the compiler ignores both and the user sees
	// nothing. Say so.
	for _, group := range f.Comments {
		for _, c := range group.List {
			if judged[c] {
				continue
			}
			if _, isDirective, _ := directive.Parse(c.Text); isDirective {
				problems = append(problems, Problem{Pos: c.Pos(), Msg: fmt.Sprintf("%s is not attached to a function declaration, so it does nothing", directive.Prefix)})
				continue
			}
			if msg, suspect := directive.Suspect(c.Text); suspect {
				problems = append(problems, Problem{Pos: c.Pos(), Msg: msg})
			}
		}
	}
	return tagged, problems
}

// File rewrites src, injecting a gate call as the first statement of every
// method carrying a //limelight:method directive.
//
// A file with no directives comes back with Count 0 and nil Source, and the
// caller should compile the original bytes untouched.
func File(filename string, src []byte) (Result, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return Result{}, err
	}

	tagged, problems := Scan(f)
	var res Result
	for _, p := range problems {
		if p.Err {
			return Result{}, fmt.Errorf("%s: %s", position(fset, p.Pos), p.Msg)
		}
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %s", position(fset, p.Pos), p.Msg))
	}
	if len(tagged) == 0 {
		return res, nil
	}

	pkgName := f.Name.Name
	var splices []splice
	for _, t := range tagged {
		splices = append(splices, splice{
			offset: fset.Position(t.Fn.Body.Lbrace).Offset + 1,
			text:   gateCall(t.Ctx, methodLabel(pkgName, t.Fn), t.Method.Fields),
		})
	}
	res.Count = len(tagged)

	local := importLocalName(f, RuntimePath, runtimePkg)
	if local == "" {
		local = genLocal
		// Spliced onto the package clause's own line: `package main; import ...`
		// is legal Go, and costs no line.
		splices = append(splices, splice{
			offset: fset.Position(f.Name.End()).Offset,
			text:   fmt.Sprintf("; import %s %q", genLocal, RuntimePath),
		})
	}
	for i := range splices {
		splices[i].text = strings.ReplaceAll(splices[i].text, "\x00", local)
	}

	res.Source = apply(src, splices)
	return res, nil
}

// gateCall builds the injected statement. \x00 stands in for the runtime's
// local import name, which is not known until every method has been scanned.
//
// The On() guard is not redundant with the check inside Emit. Emit is variadic,
// so an unguarded call makes the caller build the field-name slice before Emit
// can decide it has nothing to do; On inlines to a single atomic load and skips
// that. Measured on an M1 Pro: 4.9ns unguarded against 1.2ns guarded, per tagged
// call, for every call the switch is off — which is almost all of them.
func gateCall(ctxName, method string, fields []string) string {
	var b strings.Builder
	b.WriteString("if \x00.On() { \x00.Emit(")
	b.WriteString(ctxName)
	b.WriteString(", ")
	b.WriteString(strconv.Quote(method))
	for _, f := range fields {
		b.WriteString(", ")
		b.WriteString(strconv.Quote(f))
	}
	b.WriteString(") };")
	return b.String()
}

// methodLabel is what shows up as "method" in an event: pkg.Receiver.Method,
// or pkg.Function for a plain function. See spec/event-schema.md.
func methodLabel(pkgName string, fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return pkgName + "." + fn.Name.Name
	}
	return pkgName + "." + receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr: // generic receiver: Service[T]
		return receiverName(t.X)
	case *ast.IndexListExpr: // generic receiver: Service[K, V]
		return receiverName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return "?"
}

// contextParam returns the name of the first context.Context parameter.
//
// It reports false for a parameter named "_": there is a context, but the
// method threw it away, so there is nothing to extract from.
func contextParam(fn *ast.FuncDecl, ctxLocal string) (string, bool) {
	if ctxLocal == "" || fn.Type.Params == nil {
		return "", false
	}
	for _, p := range fn.Type.Params.List {
		sel, ok := p.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Context" {
			continue
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != ctxLocal {
			continue
		}
		for _, name := range p.Names {
			if name.Name != "_" {
				return name.Name, true
			}
		}
	}
	return "", false
}

// importLocalName returns the name path is bound to in f, or "" if f does not
// import it. pkgName is the package's own name, used when the import carries no
// explicit local name. A blank or dot import returns "" — neither gives us a
// name to call through.
func importLocalName(f *ast.File, path, pkgName string) string {
	for _, imp := range f.Imports {
		got, err := strconv.Unquote(imp.Path.Value)
		if err != nil || got != path {
			continue
		}
		if imp.Name == nil {
			return pkgName
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			continue
		}
		return imp.Name.Name
	}
	return ""
}

func apply(src []byte, splices []splice) []byte {
	sort.Slice(splices, func(i, j int) bool { return splices[i].offset > splices[j].offset })
	out := src
	for _, s := range splices {
		out = append(out[:s.offset:s.offset], append([]byte(s.text), out[s.offset:]...)...)
	}
	return out
}

func position(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}
