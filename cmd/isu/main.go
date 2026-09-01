// Command isu is an issue tracker with no database: issues are folders in the
// repository, and the pull request that fixes a bug also closes it.
package main

import (
	"io"
	"os"

	"github.com/dgorshkov/isu/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole program, parameterised over its streams and its exit code so
// that tests drive it in process rather than through a built binary. main does
// nothing but wire it to the real ones.
//
// Everything below this line lives in internal/cli, which is where the tests
// that matter are: a command that only its own binary can run is a command
// nobody tests end to end. M9-S1 stamps the version through
// -ldflags "-X github.com/dgorshkov/isu/internal/cli.Version=...".
func run(args []string, stdout, stderr io.Writer) int {
	return cli.Run(cli.Env{Args: args, Stdout: stdout, Stderr: stderr})
}
