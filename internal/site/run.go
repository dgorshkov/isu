package site

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/dgorshkov/isu/internal/cli"
)

// Sample is one command and what it printed.
type Sample struct {
	// Command is the command line as a reader would type it, without the
	// leading prompt.
	Command string
	// Output is standard output and standard error, in that order, exactly as
	// isu wrote them.
	Output string
	// Exit is the status the command left.
	Exit int
}

// isu runs one command against the fixture and returns what it printed.
//
// The command runs in process, through the same cli.Run that cmd/isu calls, so
// there is no built binary to be stale and no second place in internal/ that
// constructs a command. Its clock is Now and its environment is empty: TERM is
// unset, so the output is uncoloured for the same reason a redirect is — see
// internal/cli/render.go — and nothing the build machine exports can reach it.
func isu(dir string, args []string) Sample {
	var stdout, stderr bytes.Buffer

	code := cli.Run(cli.Env{
		Args:   args,
		Stdout: &stdout,
		Stderr: &stderr,
		Dir:    dir,
		Now:    func() time.Time { return Now },
		Getenv: func(string) string { return "" },
	})

	return Sample{
		Command: "isu " + strings.Join(args, " "),
		Output:  stdout.String() + stderr.String(),
		Exit:    code,
	}
}

// ExitError is a command whose status was not the one the document claimed.
type ExitError struct {
	Sample
	// Want is the status the document said the command would leave.
	Want int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("`%s` exited %d, not %d:\n%s",
		e.Command, e.Exit, e.Want, e.Output)
}

// OutputError is a command that printed something other than what the document
// beside it claims. It carries both so the failure names the drift rather than
// only reporting it.
type OutputError struct {
	Sample
	// Want is the output the document claims.
	Want string
}

func (e *OutputError) Error() string {
	return fmt.Sprintf("`%s` printed\n%s\nand the document claims\n%s",
		e.Command, indent(e.Output), indent(e.Want))
}

// indent offsets a block so that two of them in one message stay apart.
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "\t| " + line
	}

	return strings.Join(lines, "\n")
}
