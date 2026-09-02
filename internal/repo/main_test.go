package repo_test

import (
	"fmt"
	"os"
	"testing"
)

// asserted counts the budgets withinBudget actually held a measurement to. It
// is written from the timed tests, which the package runs one at a time, and
// read once they are all over.
var asserted int

// TestMain fails the run when perfEnv asked for the budgets and no budget was
// asserted.
//
// `make perf` selects the timed tests by name — `-run IsFast` — and a `-run`
// that matches nothing is not an error to `go test`: it prints a warning nobody
// reads and exits 0. So the one way this gate can rot is silently, by somebody
// renaming a test and leaving the target passing while it measures nothing.
// This is the assertion that the gate ran at all, and it is deliberately not a
// test function, because a test function is exactly the thing `-run` can skip.
func TestMain(m *testing.M) {
	code := m.Run()

	if code == 0 && os.Getenv(perfEnv) != "" && asserted == 0 {
		fmt.Fprintf(os.Stderr,
			"%s is set but no budget was asserted: -run selected none of the "+
				"timed tests, so this run measured nothing\n", perfEnv)

		code = 1
	}

	os.Exit(code)
}
