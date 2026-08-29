package gittest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// The harness's own refusals, driven through a counterfeit testing.TB.
//
// Every method here fails the test rather than returning an error — that is the
// harness's whole contract, and it is why scripting a fixture reads as one
// chain of statements. The cost is that the refusals cannot be exercised the
// ordinary way: calling one from a test fails that test, which is the opposite
// of asserting it happened.
//
// So the refusals get a TB that records the failure and stops, instead of a
// real one that fails this test. It is machinery, and it exists because the
// definition of done asks for 99% across the tree rather than across the
// product — a harness whose refusals were merely asserted by inspection is a
// harness whose refusals are a comment. What it buys, beyond the number, is
// that every message here has now been read once: a fixture that refuses with
// the wrong sentence is as unhelpful as one that does not refuse.

func TestTheHarnessRefusesTwoRemotes(t *testing.T) {
	require.Contains(t, refusal(t, func(tb testing.TB) {
		// WithRemote pushes the current branch, so there has to be one.
		gittest.New(tb).
			File("README.md", "# a project\n").Commit("the project").
			WithRemote().WithRemote()
	}), "already has a remote")
}

func TestTheHarnessRefusesToDetachARemoteThatWasNeverThere(t *testing.T) {
	require.Contains(t,
		refusal(t, func(tb testing.TB) { gittest.New(tb).DetachRemote() }),
		"DetachRemote before WithRemote")
}

func TestTheHarnessFailsTheTestWhenGitDoes(t *testing.T) {
	said := refusal(t, func(tb testing.TB) {
		gittest.New(tb).Git("cat-file", "-p", "0000000000000000000000000000000000000000")
	})

	require.Contains(t, said, "gittest:")
	require.Contains(t, said, "cat-file")
}

func TestTheHarnessFailsTheTestWhenAFileIsNotThere(t *testing.T) {
	require.Contains(t,
		refusal(t, func(tb testing.TB) { gittest.New(tb).ReadFile("absent.md") }),
		"reading absent.md")
}

// A directory cannot be created where a file already is, and a file cannot be
// written where a directory already is. Both are the fixture author's mistake,
// and both stop rather than leaving a half-written tree behind.
func TestTheHarnessFailsTheTestWhenAPathIsInTheWay(t *testing.T) {
	require.Contains(t, refusal(t, func(tb testing.TB) {
		gittest.New(tb).WriteFile("thing", "a file\n").WriteFile("thing/under-it", "impossible\n")
	}), "creating the directory")

	require.Contains(t, refusal(t, func(tb testing.TB) {
		r := gittest.New(tb)
		require.NoError(t, os.MkdirAll(filepath.Join(r.Dir(), "folder"), 0o750))
		r.WriteFile("folder", "impossible\n")
	}), "writing folder")
}

// A fixture asking for more issue commits than there are commits is asking for
// something that cannot be built, and hears so — the refusal added in #8, where
// the alternative was quietly building a different fixture.
func TestTheHarnessRefusesMoreTouchingCommitsThanCommits(t *testing.T) {
	said := refusal(t, func(tb testing.TB) {
		gittest.Generate(tb, gittest.Spec{Issues: 3, Commits: 5, TouchesIssues: 40})
	})

	require.Contains(t, said, "40")
	require.Contains(t, said, "only 4")
}

// refusal runs fn with a testing.TB that records the harness's refusal and
// stops there, and returns what it said.
//
// Fatalf on a real TB ends the goroutine; this ends the call with a panic the
// helper recovers, which stops the harness at the same statement and lets the
// assertion happen afterwards. Everything the harness needs that is not a
// refusal — Helper, TempDir, Cleanup — is the embedded real TB's, so a fixture
// built inside fn is a real one, cleaned up with this test.
func refusal(t *testing.T, fn func(testing.TB)) string {
	t.Helper()

	rec := &recordingTB{TB: t}

	func() {
		defer func() {
			if raised := recover(); raised != nil && raised != errRefused {
				panic(raised)
			}
		}()

		fn(rec)
	}()

	require.True(t, rec.refused,
		"the harness was expected to refuse and did not")

	return rec.said
}

// errRefused is what a recorded refusal panics with, so that a panic from
// anywhere else is not mistaken for one and swallowed.
var errRefused = fmt.Errorf("gittest refused")

type recordingTB struct {
	testing.TB

	refused bool
	said    string
}

func (r *recordingTB) Fatal(args ...any) {
	r.record(fmt.Sprint(args...))
}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.record(fmt.Sprintf(format, args...))
}

func (r *recordingTB) record(said string) {
	r.refused = true
	r.said = said

	panic(errRefused)
}
