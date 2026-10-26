package analyzer_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/stuparm/limelight/limelight-go/analyzer"
)

// Each testdata package holds one class of mistake, with a // want comment on
// every line the analyzer must report. analysistest fails on a missing
// diagnostic and on an unexpected one, so "clean" is the guard against false
// positives on prose.
//
// A directive cannot carry a trailing // want — Parse would read it as part of
// the directive — so those expectations sit on the line above, as want +1.
func TestAnalyzer(t *testing.T) {
	for _, pkg := range []string{
		"clean",
		"noctx",
		"malformed",
		"nearmiss",
		"noimport",
		"ctxonly",
		"testonly",
		"github.com/stuparm/limelight/limelight-go",
	} {
		t.Run(pkg, func(t *testing.T) {
			analysistest.Run(t, analysistest.TestData(), analyzer.Analyzer, pkg)
		})
	}
}
