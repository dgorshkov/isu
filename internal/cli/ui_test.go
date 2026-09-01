package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// `isu ui` is the one command that owns a terminal, so these run the real
// bubbletea program over a real repository and read what it wrote — the same
// end-to-end rule the rest of this package is held to, with a keyboard on the
// front of it.

// tui runs `isu ui` with a scripted keyboard and hands back what reached the
// screen.
func tui(t *testing.T, dir, typed string, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	code := Run(Env{
		Args:   append([]string{"--repo", dir, "ui"}, args...),
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader(typed),
		Dir:    dir,
		Now:    now,
		Getenv: func(string) string { return "" },
	})

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// The alt screen is what "q quits cleanly and restores the terminal" means:
// the interface draws on a second screen buffer and the terminal restores the
// first on the way out, so whatever somebody was reading before they typed
// `isu ui` is still there afterwards.
func TestTheInterfaceOpensDrawsTheBoardAndRestoresTheTerminal(t *testing.T) {
	t.Parallel()

	got := tui(t, board(t).Dir(), "q").ok(t)

	require.Contains(t, got.stdout, "\x1b[?1049h", "the interface never entered the alt screen")
	require.Contains(t, got.stdout, "\x1b[?1049l", "`q` left the terminal on the alt screen")
	require.Contains(t, got.stdout, "ISU-openly", "the board never reached the screen")
	require.Empty(t, got.stderr)
}

// The interface is the board. Two screens showing one repository that disagree
// about what is on it is the fastest way to make somebody stop believing both.
func TestTheInterfaceShowsEveryIssueTheBoardDoes(t *testing.T) {
	t.Parallel()

	r := board(t)

	drawn := tui(t, r.Dir(), "q").ok(t).stdout
	printed := isu(t, r.Dir(), "board").ok(t).stdout

	for _, id := range []string{
		"ISU-donede", "ISU-dropit", "ISU-triage",
		"ISU-inprog", "ISU-reopen", "ISU-epical", "ISU-openly",
	} {
		require.Containsf(t, printed, id, "isu board does not name %s", id)
		require.Containsf(t, drawn, id, "isu ui does not name %s", id)
	}
}

// An interface is not a thing an agent can read, and refusing to answer would
// leave one guessing. --json prints what the interface would open on, which is
// the board's own payload.
func TestTheInterfaceInJSONIsTheBoardItWouldOpenOn(t *testing.T) {
	t.Parallel()

	r := board(t)

	fromUI := decode[BoardPayload](t, isu(t, r.Dir(), "ui", "--json").ok(t))
	fromBoard := decode[BoardPayload](t, isu(t, r.Dir(), "board", "--json").ok(t))

	require.Equal(t, timeless(fromBoard), timeless(fromUI))
}

// timeless drops the two things two invocations a moment apart are allowed to
// disagree about. An age is the distance from a fixed moment to the clock, and
// the clock moves between two commands: asserting on it would be asserting that
// the test ran fast enough.
func timeless(payload BoardPayload) BoardPayload {
	payload.Freshness.AgeSeconds = 0

	for g, group := range payload.Groups {
		for i, item := range group.Issues {
			for c := range item.Claims {
				payload.Groups[g].Issues[i].Claims[c].AgeSeconds = 0
			}
		}
	}

	return payload
}

// PLAN.md M6-S2: the grouping matches `isu board` exactly. It is one function
// used twice rather than two orderings that agree, and this is the fixture that
// says so.
func TestTheInterfaceGroupsExactlyAsTheBoardDoes(t *testing.T) {
	t.Parallel()

	r := board(t)

	drawn := tui(t, r.Dir(), "q").ok(t).stdout
	payload := decode[BoardPayload](t, isu(t, r.Dir(), "board", "--json").ok(t))

	require.NotEmpty(t, payload.Groups)

	for _, group := range payload.Groups {
		require.Containsf(t, drawn, group.Status, "the interface has no %s group", group.Status)

		for _, item := range group.Issues {
			require.Containsf(t, drawn, item.ID, "%s is on the board and not on the screen", item.ID)
		}
	}
}

// A repository with nothing in it still opens. The one thing an empty board
// must not do is render as a blank pane somebody files a bug about.
func TestTheInterfaceOpensOnARepositoryWithNoIssues(t *testing.T) {
	t.Parallel()

	got := tui(t, configured(t).Dir(), "q").ok(t)

	require.Contains(t, got.stdout, "no issues")
}

// The interface reads the repository the same way every other command does, so
// it fails the same way when it cannot.
func TestTheInterfaceRefusesARefThatIsNotThere(t *testing.T) {
	t.Parallel()

	got := tui(t, board(t).Dir(), "q", "--ref", "nope")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "cannot read nope as trunk")
}

func TestTheInterfaceTakesNoArguments(t *testing.T) {
	t.Parallel()

	got := isu(t, configured(t).Dir(), "ui", "extra")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "takes no arguments")
}

// Colour is a decision about the terminal, made the same way every other
// command makes it. A frame written into a pipe is a frame with no escape
// sequences in it beyond the ones the screen itself needs.
func TestTheInterfaceIsColourlessIntoAPipe(t *testing.T) {
	t.Parallel()

	drawn := tui(t, board(t).Dir(), "q").ok(t).stdout

	require.NotContains(t, drawn, "\x1b[1m", "a pipe is not a terminal and gets no bold")
	require.NotContains(t, drawn, "\x1b[7m", "nor a reversed cursor line")
}

// The interface is reachable from a repository that has branches, which is the
// case every derived status in this product is about.
func TestTheInterfaceNamesTheRefItReadAsTrunk(t *testing.T) {
	t.Parallel()

	drawn := tui(t, board(t).Dir(), "q").ok(t).stdout

	require.Contains(t, drawn, gittest.DefaultBranch)
}

// M6-S5 · Actions from the TUI.
//
// These are the assertions that the interface runs the commands rather than
// something that looks like them: the branch a claim makes, the checkout `g`
// performs and the issue an editor files are all read back out of the
// repository afterwards, with git.

// PLAN.md M6-S5: "`c` claims through the same code path as `isu claim`". The
// proof is the repository: a claim is a branch, a state flip and a push, and
// all three are here.
func TestClaimingFromTheInterfaceIsAClaim(t *testing.T) {
	t.Parallel()

	r := board(t).WithRemote()

	// The cursor opens on the first issue of the first group, which is the one
	// resolved at trunk; `j` twice reaches an issue nobody has.
	got := tui(t, r.Dir(), "jjc").ok(t)

	require.Contains(t, got.stdout, "ISU-triage")

	require.Contains(t, r.Branches(), "isu/ISU-triage",
		"a claim is a branch, and the interface makes the same one isu claim makes")
	require.Contains(t, r.Git("show", "isu/ISU-triage:issues/ISU-triage/README.md"),
		"state: resolved")
	require.Contains(t, r.Git("log", "-1", "--format=%B", "isu/ISU-triage"), "Isu-Claim:")
}

// Losing the race is `isu claim`'s sentence, and the interface prints it rather
// than one of its own.
func TestClaimingSomethingAlreadyClaimedFromTheInterfaceNamesTheHolder(t *testing.T) {
	t.Parallel()

	r := board(t).WithRemote()

	// ISU-inprog is claimed on isu/ISU-inprog by alice, and the branch is here.
	got := tui(t, r.Dir(), "jjjc").ok(t)

	require.Contains(t, got.stdout, "already here")
	require.Contains(t, got.stdout, "alice")
}

// PLAN.md M6-S5: "`g` checks out the claiming branch."
func TestGoingToTheBranchFromTheInterfaceChecksItOut(t *testing.T) {
	t.Parallel()

	r := board(t)

	tui(t, r.Dir(), "jjjg").ok(t)

	require.Equal(t, "isu/ISU-inprog", r.Git("rev-parse", "--abbrev-ref", "HEAD"))
}

// And on an issue nobody has claimed it is a no-op with a message.
func TestGoingToTheBranchOfAnUnclaimedIssueChangesNothing(t *testing.T) {
	t.Parallel()

	r := board(t)
	before := r.Git("rev-parse", "--abbrev-ref", "HEAD")

	got := tui(t, r.Dir(), "jjg").ok(t)

	require.Contains(t, got.stdout, "nobody has claimed")
	require.Equal(t, before, r.Git("rev-parse", "--abbrev-ref", "HEAD"))
}

// PLAN.md M6-S5: "`n` opens an editor for a new issue". The editor is the
// user's own and it is handed a whole issue file to edit, so that what comes
// back is parsed by internal/issue and not by something this command invented.
func TestFilingAnIssueFromTheInterfaceOpensTheEditor(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the editor stand-in is a shell script; CI is linux and macos")
	}

	r := board(t)
	editor := writeEditor(t, `---
schema: 1
title: Filed from the interface
type: chore
state: open
owner: dmitry
created: 2026-09-01
priority: p1
---
Typed into the editor.
`)

	got := tuiIn(t, r.Dir(), map[string]string{"EDITOR": editor}, "n").ok(t)

	require.Contains(t, got.stdout, "report/")

	filed := newestIssue(t, r)
	require.Contains(t, r.ReadFile(filed), "title: Filed from the interface")
	require.Contains(t, r.ReadFile(filed), "Typed into the editor.")
	require.Contains(t, r.Git("rev-parse", "--abbrev-ref", "HEAD"), "report/")
}

// An editor that hands back something that is not an issue is told what is
// wrong with it, in internal/issue's own words.
func TestAnEditorThatWritesSomethingInvalidIsToldWhy(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the editor stand-in is a shell script; CI is linux and macos")
	}

	r := board(t)
	editor := writeEditor(t, "this is not an issue file at all\n")

	got := tuiIn(t, r.Dir(), map[string]string{"EDITOR": editor}, "n").ok(t)

	require.Contains(t, got.stdout, "will not decode")
}

// An editor that writes nothing filed nothing, which is how somebody changes
// their mind.
func TestAnEditorThatWritesNothingFilesNothing(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the editor stand-in is a shell script; CI is linux and macos")
	}

	r := board(t)
	before := r.Branches()

	got := tuiIn(t, r.Dir(), map[string]string{"EDITOR": writeEditor(t, "")}, "n").ok(t)

	require.Contains(t, got.stdout, "nothing was written")
	require.Equal(t, before, r.Branches())
}

// tuiIn is `isu ui` with an environment, which is how the editor is named.
func tuiIn(t *testing.T, dir string, env map[string]string, typed string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	code := Run(Env{
		Args:   []string{"--repo", dir, "ui"},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader(typed + "q"),
		Dir:    dir,
		Now:    now,
		Getenv: func(name string) string { return env[name] },
	})

	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// newestIssue is the README of the issue folder this repository did not have
// before, which is how a test reads back an id it did not choose.
func newestIssue(t *testing.T, r *gittest.Repo) string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(r.Dir(), "issues"))
	require.NoError(t, err)

	for _, entry := range entries {
		if !slices.Contains([]string{
			"ISU-epical", "ISU-openly", "ISU-inprog", "ISU-reopen",
			"ISU-donede", "ISU-dropit", "ISU-triage",
		}, entry.Name()) {
			return "issues/" + entry.Name() + "/README.md"
		}
	}

	require.Fail(t, "no issue was filed")

	return ""
}
