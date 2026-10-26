// Package analyzer is the go/analysis pass that reports a //limelight:method
// directive which cannot take effect.
//
// A directive is a comment, so nothing in the Go toolchain objects to one that
// cannot work: a tag on a method with no context.Context, a directive with a
// space after the //, a package that never imports the runtime. Each of them
// builds, ships, and emits nothing on the day someone flips the switch.
//
// The analyzer does not decide for itself whether a tag works. It reports what
// rewrite.Scan reports — the same function the -toolexec shim instruments from
// — so the analyzer and the shim cannot disagree about a tag. The import check
// is its own, because the shim reads the package's importcfg, which vet never
// sees.
package analyzer

import (
	"go/ast"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/stuparm/limelight/limelight-go/internal/rewrite"
)

// Analyzer reports //limelight:method directives that cannot take effect.
var Analyzer = &analysis.Analyzer{
	Name: "limelight",
	Doc: `report //limelight:method directives that cannot take effect

A //limelight:method directive is a comment: when it is malformed, misspelled,
misplaced, or on a method the rewriter cannot instrument, the build succeeds and
the tag emits nothing. This analyzer reports each of those — the same verdicts
the -toolexec shim reaches — and a package whose tagged methods the shim would
refuse because it does not import the limelight runtime.`,
	URL: "https://github.com/stuparm/limelight",
	Run: run,
	// Every check is syntactic, so a package that does not type-check yet —
	// mid-edit, in gopls — still gets its diagnostics.
	RunDespiteErrors: true,
}

func run(pass *analysis.Pass) (any, error) {
	// The shim never instruments the runtime itself, so nothing in it is a tag.
	if pass.Pkg.Path() == rewrite.RuntimePath {
		return nil, nil
	}

	// go build compiles the non-test files alone, so a tag there needs the
	// runtime imported by a non-test file. A tag in a test file is compiled
	// alongside every file of the package, so any import will do.
	var importedByNonTest, importedByAny bool
	for _, f := range pass.Files {
		if importsRuntime(f) {
			importedByAny = true
			importedByNonTest = importedByNonTest || !isTestFile(pass, f)
		}
	}

	for _, f := range pass.Files {
		tagged, problems := rewrite.Scan(f)
		for _, p := range problems {
			pass.Report(analysis.Diagnostic{Pos: p.Pos, Message: p.Msg})
		}
		if len(tagged) == 0 {
			continue // the shim only demands the import when it has something to instrument
		}
		imported := importedByNonTest
		if isTestFile(pass, f) {
			imported = importedByAny
		}
		if !imported {
			// Once per file: enough to show in whichever file is open, without
			// a line per tag in a package with hundreds of them.
			pass.Reportf(tagged[0].Directive.Pos(), "package %s has tagged methods but %s", pass.Pkg.Path(), rewrite.MissingImport)
		}
	}
	return nil, nil
}

// importsRuntime reports whether f imports the runtime under any name. A blank
// import counts — it is the documented way to satisfy the shim — and so does a
// dot import, which the shim instruments through.
func importsRuntime(f *ast.File) bool {
	for _, imp := range f.Imports {
		if path, err := strconv.Unquote(imp.Path.Value); err == nil && path == rewrite.RuntimePath {
			return true
		}
	}
	return false
}

func isTestFile(pass *analysis.Pass, f *ast.File) bool {
	return strings.HasSuffix(pass.Fset.File(f.FileStart).Name(), "_test.go")
}
