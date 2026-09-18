// Package toolexec is the -toolexec driver: the shim the go command invokes in
// place of each toolchain program.
//
// The go command runs it as `limelight toolexec <realtool> <args...>`. Every
// invocation it does not care about is passed straight through, so a build with
// limelight wired in behaves exactly like one without it until a file carries a
// //limelight:method directive.
package toolexec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/stuparm/limelight/limelight-go/internal/rewrite"
)

// Run executes one toolchain invocation and returns its exit code. args is the
// real tool path followed by the arguments the go command chose.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "limelight: toolexec needs a tool to run")
		return 2
	}
	tool, toolArgs := args[0], args[1:]

	// The go command asks every tool for its version and folds the answer into
	// the build cache key. Appending our own identity is what makes the cache
	// notice that an instrumented build differs from a plain one — and, because
	// the identity hashes this binary, what makes it notice when the rewriter
	// itself changes.
	if isVersionQuery(toolArgs) {
		return runVersion(tool, toolArgs)
	}

	if toolName(tool) == "compile" {
		rewritten, cleanup, err := instrument(toolArgs)
		defer cleanup()
		if err != nil {
			fmt.Fprintf(os.Stderr, "limelight: %v\n", err)
			return 1
		}
		toolArgs = rewritten
	}

	return run(tool, toolArgs)
}

// instrument rewrites any source file carrying a directive and returns args
// pointing at the rewritten copies.
func instrument(args []string) (out []string, cleanup func(), err error) {
	cleanup = func() {}

	pkg := flagValue(args, "-p")
	if pkg == rewrite.RuntimePath {
		return args, cleanup, nil // never instrument the runtime itself
	}

	var sources []int
	for i, a := range args {
		if strings.HasSuffix(a, ".go") && !strings.HasPrefix(a, "-") {
			sources = append(sources, i)
		}
	}
	if len(sources) == 0 {
		return args, cleanup, nil
	}

	results := make(map[int]rewrite.Result)
	total := 0
	for _, i := range sources {
		src, rerr := os.ReadFile(args[i])
		if rerr != nil {
			return nil, cleanup, fmt.Errorf("reading %s: %w", args[i], rerr)
		}
		if !bytes.Contains(src, []byte("//limelight:")) {
			continue // the overwhelmingly common case: no parse, no cost
		}
		res, rerr := rewrite.File(args[i], src)
		if rerr != nil {
			return nil, cleanup, rerr
		}
		for _, w := range res.Warnings {
			fmt.Fprintf(os.Stderr, "limelight: %s\n", w)
		}
		if res.Count > 0 {
			results[i] = res
			total += res.Count
		}
	}
	if total == 0 {
		return args, cleanup, nil
	}

	// The injected call needs the runtime in this package's dependency graph.
	// The go command built importcfg before we were invoked and will not add to
	// it, so a package that does not already import limelight cannot be
	// instrumented. Refusing loudly beats emitting code that cannot compile.
	if !importable(flagValue(args, "-importcfg")) {
		return nil, cleanup, fmt.Errorf(
			"package %s has %d tagged method(s) but does not import %s.\n"+
				"       -toolexec cannot add to the build graph. Add an import to any file in the package:\n"+
				"           import _ %q",
			pkg, total, rewrite.RuntimePath, rewrite.RuntimePath)
	}

	dir, derr := os.MkdirTemp("", "limelight-")
	if derr != nil {
		return nil, cleanup, derr
	}
	cleanup = func() { os.RemoveAll(dir) }

	out = append(out, args...)
	for i, res := range results {
		orig, aerr := filepath.Abs(args[i])
		if aerr != nil {
			orig = args[i]
		}
		// A //line directive maps every position in the rewritten copy back to
		// the user's file, so panics and debuggers never mention a temp path.
		body := append([]byte(fmt.Sprintf("//line %s:1\n", orig)), res.Source...)

		path := filepath.Join(dir, filepath.Base(args[i]))
		if werr := os.WriteFile(path, body, 0o600); werr != nil {
			return nil, cleanup, werr
		}
		out[i] = path
		verbosef("instrumented %d method(s) in %s", res.Count, orig)
	}
	return out, cleanup, nil
}

// importable reports whether the limelight runtime is in the package's
// importcfg — that is, whether the go command already resolved it as a
// dependency of the package being compiled.
func importable(importcfg string) bool {
	if importcfg == "" {
		return false
	}
	data, err := os.ReadFile(importcfg)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		for _, verb := range []string{"packagefile ", "packageshlib "} {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), verb); ok {
				if path, _, found := strings.Cut(rest, "="); found && path == rewrite.RuntimePath {
					return true
				}
			}
		}
	}
	return false
}

// flagValue reads a flag in either the "-p main" or "-p=main" form.
func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
	}
	return ""
}

func isVersionQuery(args []string) bool {
	for _, a := range args {
		if a == "-V=full" {
			return true
		}
	}
	return false
}

func runVersion(tool string, args []string) int {
	cmd := exec.Command(tool, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		if code, ok := exitCode(err); ok {
			return code
		}
		fmt.Fprintf(os.Stderr, "limelight: %s -V=full: %v\n", tool, err)
		return 1
	}
	fmt.Printf("%s +limelight:%s\n", strings.TrimRight(string(out), "\n"), selfID())
	return 0
}

func run(tool string, args []string) int {
	cmd := exec.Command(tool, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if code, ok := exitCode(err); ok {
			return code
		}
		fmt.Fprintf(os.Stderr, "limelight: running %s: %v\n", tool, err)
		return 1
	}
	return 0
}

func exitCode(err error) (int, bool) {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), true
	}
	return 0, false
}

// toolName strips the directory and any .exe suffix, so "compile" matches on
// every platform.
func toolName(tool string) string {
	return strings.TrimSuffix(filepath.Base(tool), ".exe")
}

var (
	idOnce sync.Once
	id     string
)

// selfID hashes this binary. It rides in the -V=full answer so that rebuilding
// the rewriter invalidates every package it instrumented — without it, editing
// the rewriter and re-running would silently reuse cached, stale object files.
func selfID() string {
	idOnce.Do(func() {
		id = "unknown"
		exe, err := os.Executable()
		if err != nil {
			return
		}
		f, err := os.Open(exe)
		if err != nil {
			return
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return
		}
		id = hex.EncodeToString(h.Sum(nil))[:12]
	})
	return id
}

func verbosef(format string, args ...any) {
	if os.Getenv("LIMELIGHT_VERBOSE") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "limelight: "+format+"\n", args...)
}
