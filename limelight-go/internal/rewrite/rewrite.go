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

	var res Result
	var splices []splice

	pkgName := f.Name.Name
	ctxLocal := importLocalName(f, "context", "context")
	tagged := map[*ast.CommentGroup]bool{}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		m, found, derr := parseDoc(fn.Doc, tagged)
		if derr != nil {
			return Result{}, fmt.Errorf("%s: %w", position(fset, fn.Doc.Pos()), derr)
		}
		if !found {
			continue
		}
		if fn.Body == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %s on %s, which has no body", position(fset, fn.Pos()), directive.Prefix, fn.Name.Name))
			continue
		}
		ctxName, ok := contextParam(fn, ctxLocal)
		if !ok {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %s on %s, which takes no context.Context — nothing to extract from", position(fset, fn.Pos()), directive.Prefix, fn.Name.Name))
			continue
		}

		splices = append(splices, splice{
			offset: fset.Position(fn.Body.Lbrace).Offset + 1,
			text:   gateCall(ctxName, methodLabel(pkgName, fn), m.Fields),
		})
		res.Count++
	}

	// A directive that is not a function's doc comment is inert: the compiler
	// ignores it and the user sees nothing. Say so.
	for _, group := range f.Comments {
		if tagged[group] {
			continue
		}
		for _, c := range group.List {
			if _, isDirective, _ := directive.Parse(c.Text); isDirective {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %s is not attached to a function declaration, so it does nothing", position(fset, c.Pos()), directive.Prefix))
			}
		}
	}

	if res.Count == 0 {
		return res, nil
	}

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

// parseDoc finds a limelight directive in a doc comment group.
func parseDoc(doc *ast.CommentGroup, tagged map[*ast.CommentGroup]bool) (directive.Method, bool, error) {
	for _, c := range doc.List {
		m, isDirective, err := directive.Parse(c.Text)
		if !isDirective {
			continue
		}
		tagged[doc] = true
		if err != nil {
			return directive.Method{}, true, err
		}
		return m, true, nil
	}
	return directive.Method{}, false, nil
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
