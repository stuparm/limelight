// Command limelight-vet reports //limelight:method directives that cannot take
// effect — the mistakes that otherwise build cleanly and emit nothing.
//
//	go install github.com/stuparm/limelight/limelight-go/analyzer/cmd/limelight-vet
//	go vet ./... && go vet -vettool=$(which limelight-vet) ./...
//
// -vettool replaces the standard vet passes rather than adding to them, which
// is why plain go vet runs first. It also runs standalone: limelight-vet ./...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/stuparm/limelight/limelight-go/analyzer"
)

func main() { singlechecker.Main(analyzer.Analyzer) }
