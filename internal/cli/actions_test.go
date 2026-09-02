package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/ui"
)

// The actions internal/ui is handed, driven directly.
//
// `isu ui` runs them through a keyboard and a terminal, and those tests are in
// ui_test.go. These are the answers a key cannot easily ask for: the ref that
// will not read, the id that is not there, the folder something else is sitting
// on. Every one of them is a sentence somebody sees in the footer, so every one
// of them is worth having read once.

// wiring builds the actions against a real repository, at a ref the caller
// chooses — naming one that is not there is how a test reaches the failures a
// keyboard cannot produce.
func wiring(t *testing.T, r *gittest.Repo, ref string) *actions {
	t.Helper()

	a := &app{
		env: Env{
			Stdout: io.Discard, Stderr: io.Discard, Dir: r.Dir(),
			Now: now, Getenv: func(string) string { return "" },
		},
		repoPath: r.Dir(),
		ref:      ref,
	}

	s, err := a.open(t.Context())
	require.NoError(t, err)

	return &actions{app: a, session: s}
}

func TestTheInterfaceReReadsTheRepositoryAfterAnAction(t *testing.T) {
	t.Parallel()

	data, err := wiring(t, board(t), "").Reload()
	require.NoError(t, err)

	require.Equal(t, gittest.DefaultBranch, data.Trunk)
	require.NotEmpty(t, data.Groups)
	require.NotNil(t, data.Board)

	found := 0
	for _, group := range data.Groups {
		found += len(group.Items)
	}

	require.Equal(t, 7, found, "the same seven issues isu board reads")
	require.Contains(t, data.Freshness, "remote refs")
}

// Every action starts by deriving the repository, so every action fails the
// same way when it cannot. The sentence is the one `isu board` gives, because
// it is the same function that produced it.
func TestEveryActionSaysWhenTheRefWillNotRead(t *testing.T) {
	t.Parallel()

	act := wiring(t, board(t), "nope")

	_, err := act.Reload()
	require.ErrorContains(t, err, "cannot read nope as trunk")

	_, err = act.Folder("ISU-openly")
	require.ErrorContains(t, err, "cannot read nope as trunk")

	_, err = act.Claim("ISU-openly")
	require.ErrorContains(t, err, "cannot read nope as trunk")

	_, err = act.New(ui.Streams{})
	require.ErrorContains(t, err, "cannot read nope as trunk")
}

func TestTheFolderActionRefusesAnIdTheBoardDoesNotHold(t *testing.T) {
	t.Parallel()

	_, err := wiring(t, board(t), "").Folder("ISU-nobody")

	require.ErrorContains(t, err, "no issue ISU-nobody")
}

func TestTheFolderActionCarriesWhatLivesBesideTheIssue(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq",
			gittest.Attachment("repro.har", "{}\n"),
			gittest.Comment("2026-09-01-alice-01.md", "It happens on Firefox too.\n")).
		Commit("report an issue with things beside it")

	held, err := wiring(t, r, "").Folder("ISU-7f3akq")
	require.NoError(t, err)

	require.Equal(t, []string{"repro.har"}, held.Attachments)
	require.Len(t, held.Comments, 1)
	require.Equal(t, "2026-09-01-alice-01.md", held.Comments[0].Name)
	require.Contains(t, held.Comments[0].Body, "Firefox")
}

// A folder that cannot be listed is reported rather than rendered as an issue
// with nothing beside it. The lever is the one PLAN.md's coverage note names —
// a path something else is already sitting on.
func TestTheFolderActionSaysWhenTheFolderWillNotOpen(t *testing.T) {
	t.Parallel()

	r := configured(t).Issue("ISU-7f3akq").Commit("report one issue")

	dir := filepath.Join(r.Dir(), "issues", "ISU-7f3akq")
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("not a folder\n"), 0o600))

	_, err := wiring(t, r, "").Folder("ISU-7f3akq")

	require.ErrorContains(t, err, "issues/ISU-7f3akq")
}

// Claiming through the interface is `isu claim`, and it refuses what `isu
// claim` refuses, in the same words.
func TestTheClaimActionRefusesAFinishedIssue(t *testing.T) {
	t.Parallel()

	_, err := wiring(t, board(t), "").Claim("ISU-donede")

	require.ErrorContains(t, err, "already done")
}

func TestTheClaimActionReportsWhatItWrote(t *testing.T) {
	t.Parallel()

	said, err := wiring(t, board(t).WithRemote(), "").Claim("ISU-reopen")
	require.NoError(t, err)

	require.Contains(t, said, "ISU-reopen")
	require.Contains(t, said, "on isu/ISU-reopen")
	require.Contains(t, said, "commit ")
	require.Contains(t, said, "pushed")
}

func TestTheGotoActionRefusesAnIssueNobodyHasClaimed(t *testing.T) {
	t.Parallel()

	_, err := wiring(t, board(t), "").Goto("ISU-openly")

	require.ErrorContains(t, err, "nobody has claimed ISU-openly from this clone")
}

func TestTheGotoActionChecksTheClaimingBranchOut(t *testing.T) {
	t.Parallel()

	r := board(t)

	said, err := wiring(t, r, "").Goto("ISU-inprog")
	require.NoError(t, err)

	require.Equal(t, "on isu/ISU-inprog", said)
	require.Equal(t, "isu/ISU-inprog", r.Git("rev-parse", "--abbrev-ref", "HEAD"))
}

// rendererFor is the same decision themeFor makes, and this is the half of it a
// buffer cannot reach: a character device that is not a pipe, with a terminal
// name set, may have colour.
func TestTheInterfaceTakesColourFromWhatItIsWritingTo(t *testing.T) {
	t.Parallel()

	device, err := osOpenDevNull()
	require.NoError(t, err)

	defer func() { _ = device.Close() }()

	a := &app{env: Env{Getenv: func(name string) string {
		if name == "TERM" {
			return "xterm-256color"
		}

		return ""
	}}}

	// /dev/null is as close to a terminal as a test suite gets: a character
	// device, which is the signal themeFor reads. What lipgloss then does with
	// it is lipgloss's decision from the same signal, and into anything that is
	// not really a terminal it renders no escape sequences at all — which is
	// what the golden frames in internal/ui depend on.
	require.NotNil(t, a.rendererFor(device))
	require.NotContains(t, a.rendererFor(io.Discard).NewStyle().Bold(true).Render("x"),
		"\x1b", "a pipe gets no escape sequences")
}

// The interface reads the keyboard from the process's own input unless a test
// hands it one, which is the same rule every other stream in Env follows.
func TestTheInterfaceReadsTheProcessesOwnInputByDefault(t *testing.T) {
	t.Parallel()

	require.Equal(t, os.Stdin, Env{}.stdin())
	require.NotEqual(t, os.Stdin, Env{Stdin: io.LimitReader(os.Stdin, 0)}.stdin())
}

// And it fails to open the way every other command fails to open.
func TestTheInterfaceSaysWhenThereIsNoRepository(t *testing.T) {
	t.Parallel()

	got := tui(t, t.TempDir(), "q")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "not inside a git working tree")
}
