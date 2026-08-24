// Command isu is an issue tracker with no database: issues are folders in the
// repository, and the pull request that fixes a bug also closes it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// version is the build version. Release builds override it via
// -ldflags "-X main.version=..." (M9-S1); anything else is a development
// build and says so.
var version = "0.1.0-dev"

const usage = `isu — an issue tracker with no database

Usage:
  isu [flags]

Flags:
  -h, --help      show this help and exit
      --version   show the version and exit
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole program, parameterised over its streams and its exit code so
// that tests drive it in process rather than through a built binary. main does
// nothing but wire it to the real ones.
//
// The command surface is stdlib flag on purpose: cobra arrives at M4-S1 and
// replaces all of this, so there is nothing here worth building twice.
//
// Write errors on stdout and stderr are discarded throughout: when the stream
// we would report the failure on is the stream that just failed, there is
// nothing useful left to do.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("isu", flag.ContinueOnError)
	// flag's own reporting cannot tell help from error, and puts both on the
	// same stream. Silence it and decide here instead.
	fs.SetOutput(io.Discard)

	showVersion := fs.Bool("version", false, "show the version and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(stdout, usage)
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "isu: %v\n\n%s", err, usage)
		return 2
	}

	if *showVersion {
		_, _ = fmt.Fprintln(stdout, version)
		return 0
	}

	// Until there are subcommands, a bare invocation has nothing to do; say so
	// on stderr so that `isu | ...` never sees usage text as data.
	_, _ = fmt.Fprint(stderr, usage)
	return 2
}
