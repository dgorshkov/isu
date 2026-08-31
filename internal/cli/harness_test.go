package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// This is the end-to-end harness PLAN.md M4-S1 asks for, and it ships with the
// first command rather than after the last one. Everything through M3 is tested
// end to end through the stack — a real git repository, no mocks anywhere — but
// nothing was tested end to end through the *product*, because there was no
// product. A milestone that builds eight commands and writes its first
// end-to-end test at M4-S8 has seven commands nobody ever ran.
//
// Nothing here is a mock. A test builds a real repository with gittest, runs a
// real command against it in process, and reads what it printed. Only the
// clock, the environment and the streams are arguments; git and the repository
// are the two things that stay real.

// Commands in these tests run on the real clock, and that is not a shortcut.
//
// Everything a fixture dates — a claim aged past stale_days, a remote ref
// fetched three days ago, an issue's created line — is dated by gittest
// relative to the real clock, because git stamps a commit with a date and not
// with an offset. A test clock pinned to a date in the calendar would put every
// one of those ages a variable distance from the moment they are measured
// against, and the distance would be however long ago this file was written:
// a claim aged three days would read as three days old on the afternoon this
// was written and as four hundred days old a year later.
//
// What that costs is golden files with a timestamp in them, so golden scrubs
// the two things that cannot be stable: RFC 3339 moments and object ids.
func now() time.Time { return time.Now() }

// today is the date isu stamps on anything it creates during a test.
func today() string { return now().UTC().Format(time.DateOnly) }

// result is one invocation of isu.
type result struct {
	code   int
	stdout string
	stderr string
}

// ok fails the test unless the command succeeded, quoting stderr — a failure
// whose message is "expected 0, got 1" is a failure nobody can act on.
func (r result) ok(t *testing.T) result {
	t.Helper()
	require.Equalf(t, 0, r.code, "isu failed: %s", r.stderr)

	return r
}

// isu runs one command against a repository, as a person would.
func isu(t *testing.T, dir string, args ...string) result {
	t.Helper()

	return isuIn(t, dir, nil, args...)
}

// isuIn is isu with an environment. The map is the whole environment the
// command can see, so a test says what matters and nothing leaks in from the
// machine it runs on.
func isuIn(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	code := Run(Env{
		Args:   append([]string{"--repo", dir}, args...),
		Stdout: &stdout,
		Stderr: &stderr,
		Dir:    dir,
		Now:    now,
		Getenv: func(name string) string { return env[name] },
	})

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// decode reads the one JSON value a command printed.
func decode[T any](t *testing.T, r result) T {
	t.Helper()

	var value T

	dec := json.NewDecoder(strings.NewReader(r.stdout))
	// A field the documented struct does not have is a contract the code and
	// the document disagree about, and it should fail here rather than in
	// somebody's pipeline.
	dec.DisallowUnknownFields()

	require.NoErrorf(t, dec.Decode(&value), "output was not the documented shape: %s", r.stdout)

	return value
}

// decodeLines reads the newline-delimited JSON `ready` prints.
func decodeLines[T any](t *testing.T, r result) []T {
	t.Helper()

	var values []T

	for line := range strings.SplitSeq(strings.TrimSpace(r.stdout), "\n") {
		if line == "" {
			continue
		}

		var value T

		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		require.NoErrorf(t, dec.Decode(&value), "not the documented shape: %s", line)

		values = append(values, value)
	}

	return values
}

// configured is an empty repository isu will talk to: one commit, and the one
// line of configuration every command needs.
func configured(t *testing.T) *gittest.Repo {
	t.Helper()

	return gittest.New(t).
		File(".isu.yml", "prefix: ISU\n").
		File("README.md", "# a repository\n").
		Commit("set isu up")
}

// board is a repository holding an issue in every status the table in PLAN.md
// §1 names, which is what a renderer has to be correct about.
//
//	ISU-donede  done            resolved at trunk
//	ISU-dropit  dropped         dropped at trunk
//	ISU-triage  awaiting triage on a branch, never on trunk
//	ISU-inprog  in progress     claimed on isu/ISU-inprog
//	ISU-reopen  reopened        resolved once at trunk, open now
//	ISU-openly  open            on trunk, nobody on it
//	ISU-epical  epic            parent of ISU-openly
func board(t *testing.T) *gittest.Repo {
	t.Helper()

	r := configured(t).
		Backdate(30).
		Issue("ISU-epical", gittest.Type("epic"), gittest.Without("state"),
			gittest.Title("Make login reliable"), gittest.Owner("dmitry")).
		Issue("ISU-openly", gittest.Title("Login retries drop the second attempt"),
			gittest.Type("bug"), gittest.Owner("dmitry"), gittest.Priority("p1"),
			gittest.Parent("ISU-epical"), gittest.Field("repro", "post twice"),
			gittest.Body("The second POST is dropped.\n")).
		Issue("ISU-inprog", gittest.Title("Board renders epics"), gittest.Type("story"),
			gittest.Owner("alice"), gittest.Field("acceptance", "the epic shows its children")).
		Issue("ISU-reopen", gittest.Title("Upgrade the linter"), gittest.Type("chore"),
			gittest.Owner("alice")).
		Issue("ISU-donede", gittest.Title("What does contention cost?"), gittest.Type("spike"),
			gittest.Owner("dmitry"), gittest.Field("question", "how much?")).
		Issue("ISU-dropit", gittest.Title("Rewrite it in another language"),
			gittest.Type("chore"), gittest.Owner("dmitry")).
		Commit("report the first six issues")

	// done, and dropped.
	r.Backdate(20).
		Issue("ISU-donede", gittest.Type("spike"), gittest.Owner("dmitry"),
			gittest.Title("What does contention cost?"), gittest.Field("question", "how much?"),
			gittest.State("resolved")).
		Issue("ISU-dropit", gittest.Type("chore"), gittest.Owner("dmitry"),
			gittest.Title("Rewrite it in another language"), gittest.State("dropped"),
			gittest.Field("reason", "we like this one"), gittest.Field("resolution", "wontfix")).
		Commit("finish two")

	// reopened: resolved at an earlier trunk commit, open now.
	r.Backdate(15).
		Issue("ISU-reopen", gittest.Type("chore"), gittest.Owner("alice"),
			gittest.Title("Upgrade the linter"), gittest.State("resolved")).
		Commit("resolve ISU-reopen").
		Backdate(10).
		Issue("ISU-reopen", gittest.Type("chore"), gittest.Owner("alice"),
			gittest.Title("Upgrade the linter"), gittest.State("open")).
		Commit("the linter upgrade did not hold")

	// in progress: a claim, which is a branch flipping the state.
	r.Backdate(3).As("alice").
		Branch("isu/ISU-inprog").Checkout("isu/ISU-inprog").
		Issue("ISU-inprog", gittest.Type("story"), gittest.Owner("alice"),
			gittest.Title("Board renders epics"),
			gittest.Field("acceptance", "the epic shows its children"),
			gittest.State("resolved")).
		Commit("claim ISU-inprog").
		Checkout(gittest.DefaultBranch).As("")

	// awaiting triage: a folder on a branch trunk has never seen.
	r.Backdate(1).
		Branch("report/ISU-triage").Checkout("report/ISU-triage").
		Issue("ISU-triage", gittest.Title("Sign-up page 500s on Firefox"),
			gittest.Type("bug"), gittest.Owner("support"), gittest.Priority("p0"),
			gittest.Field("repro", "sign up on Firefox")).
		Commit("report ISU-triage").
		Checkout(gittest.DefaultBranch).
		Backdate(0)

	return r
}

// golden compares output with a recorded file, and rewrites it under -update.
//
// The renderers in this package are the product's face, and a diff of what they
// print is the only review that catches a column that no longer lines up.
func golden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	got = scrub(got)

	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))

		return
	}

	want, err := os.ReadFile(path)
	require.NoErrorf(t, err, "no golden file: run `go test ./internal/cli -update`")
	require.Equal(t, string(want), got)
}

// osOpenDevNull opens something that is a character device and is not a
// terminal, which is as close as a test suite gets to one.
func osOpenDevNull() (*os.File, error) {
	return os.OpenFile(os.DevNull, os.O_WRONLY, 0)
}

// The two things in isu's output that cannot be the same twice: a moment, and
// an object id. Everything else in a golden file is the review.
var (
	moments = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})`)
	oids    = regexp.MustCompile(`\b[0-9a-f]{8,40}\b`)
)

// scrub replaces what a golden file cannot pin down.
//
// It is deliberately narrow. A renderer's columns, its wording, its ordering
// and the facts it chooses to print are all the review; the timestamp of a
// commit written eight milliseconds ago is not.
func scrub(s string) string {
	s = moments.ReplaceAllString(s, "<when>")

	return oids.ReplaceAllString(s, "<oid>")
}
