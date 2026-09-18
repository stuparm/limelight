// Command limelight is the -toolexec shim that makes a //limelight:method
// directive do something.
//
// Wire it into a build and the tag takes effect with no source left rewritten
// on disk:
//
//	go build -o limelight ./cmd/limelight
//	go run -toolexec="$PWD/limelight toolexec" .
//
// Without it the directive is an ordinary comment and the compiler ignores it,
// which is the failure mode the vet analyzer exists to catch.
package main

import (
	"fmt"
	"os"

	"github.com/stuparm/limelight/limelight-go/internal/toolexec"
)

const usage = `limelight — on-demand, identity-targeted method tracing

Usage:
  limelight toolexec <tool> [args...]   the -toolexec shim; the go command
                                        invokes this, you normally do not

  go build -o limelight ./cmd/limelight
  go run -toolexec="$PWD/limelight toolexec" .

Environment:
  LIMELIGHT_VERBOSE=1   report each method instrumented, on stderr
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "toolexec":
		os.Exit(toolexec.Run(os.Args[2:]))
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "limelight: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
