package ui_test

import (
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/ui"
)

// M6-S5 · Actions from the TUI.
//
// "Done when the TUI shares command implementations with the CLI, proven by a
// test that fails if the TUI package calls git directly." The proof is the last
// test in this file, and the design that makes it easy to pass is the one every
// test above it relies on: this package is handed everything it draws, and
// everything it cannot do itself goes back out through Actions.

// wired is a fake Actions that records what it was asked to do and answers with
// what a test wants. It is not a mock of git — nothing in this project mocks
// git — it is a stand-in for internal/cli, which is where the git is.
type wired struct {
	mu sync.Mutex

	held      ui.Folder
	folderErr error

	claimed  []string
	wentTo   []string
	filed    int
	reloaded int

	claimSaid string
	claimErr  error
	gotoSaid  string
	gotoErr   error
	newSaid   string
	newErr    error
	reloadErr error

	// onNew is what the editor does with the terminal it was handed.
	onNew func(ui.Streams)
}

func (w *wired) Folder(string) (ui.Folder, error) { return w.held, w.folderErr }

func (w *wired) Claim(id string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.claimed = append(w.claimed, id)

	return w.claimSaid, w.claimErr
}

func (w *wired) Goto(id string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.wentTo = append(w.wentTo, id)

	return w.gotoSaid, w.gotoErr
}

func (w *wired) New(streams ui.Streams) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.filed++

	if w.onNew != nil {
		w.onNew(streams)
	}

	return w.newSaid, w.newErr
}

func (w *wired) Reload() (ui.Data, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.reloaded++

	if w.reloadErr != nil {
		return ui.Data{}, w.reloadErr
	}

	return board().Data, nil
}

// did is the condition a script waits on when the screen is not something to
// synchronise on: the fake has been asked to do this much.
func (w *wired) did(actions, reloads int) func() bool {
	return func() bool {
		claims, gotos, files, reloaded := w.count()

		return claims+gotos+files >= actions && reloaded >= reloads
	}
}

func (w *wired) count() (claims, gotos, files, reloads int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return len(w.claimed), len(w.wentTo), w.filed, w.reloaded
}

// acting is the board with something wired to it.
func acting(actions ui.Actions) ui.Input {
	in := board()
	in.Actions = actions

	return in
}

func TestClaimingSaysWhatHappenedAndReReadsTheBoard(t *testing.T) {
	t.Parallel()

	w := &wired{claimSaid: "ISU-donede claimed on isu/ISU-donede, pushed"}

	frame := press(t, acting(w), 110, 24, []string{"c"}, "pushed")

	require.Contains(t, frame, "ISU-donede claimed on isu/ISU-donede, pushed")

	claims, _, _, reloads := w.count()
	require.Equal(t, 1, claims)
	require.Equal(t, []string{"ISU-donede"}, w.claimed, "the issue the cursor was on")
	require.Positive(t, reloads, "a claim changes the board, so the board is read again")
}

// M6-S5: "claiming an already-claimed issue shows the holder". The
// sentence is `isu claim`'s own, because it is `isu claim` that produced it.
func TestClaimingSomethingSomebodyElseHasShowsWhoHasIt(t *testing.T) {
	t.Parallel()

	w := &wired{claimErr: errors.New(
		"ISU-donede is already claimed: alice pushed isu/ISU-donede first, " +
			"so the work is theirs")}

	require.Contains(t, press(t, acting(w), 140, 24, []string{"c"}, "alice"), "alice")
}

// M6-S5: "`g` on an unclaimed issue is a no-op with a message".
func TestGoingToTheBranchOfAnUnclaimedIssueSaysSo(t *testing.T) {
	t.Parallel()

	w := &wired{gotoErr: errors.New(
		"there is no isu/ISU-donede here: nobody has claimed ISU-donede from this clone")}

	before := sized(t, acting(w), 140, 24)
	after := press(t, acting(w), 140, 24, []string{"g"}, "nobody has claimed")

	require.Contains(t, after, "nobody has claimed ISU-donede from this clone")
	require.Equal(t, drawnIDs(before.View()), drawnIDs(after), "and the list did not move")
}

func TestGoingToTheBranchOfAClaimedIssueChecksItOut(t *testing.T) {
	t.Parallel()

	w := &wired{gotoSaid: "on isu/ISU-inprog"}

	require.Contains(t, press(t, acting(w), 110, 24, []string{"g"}, "on isu/ISU-inprog"),
		"on isu/ISU-inprog")
	require.Equal(t, []string{"ISU-donede"}, w.wentTo)
}

// M6-S5: "the editor is injected and tested with a fake". The terminal
// goes with it: an editor that cannot read the keyboard is an editor nobody can
// type into, so the interface releases the screen for as long as it runs.
func TestFilingAnIssueHandsTheEditorTheTerminal(t *testing.T) {
	var got ui.Streams

	w := &wired{
		newSaid: "ISU-newone1 on report/ISU-newone1",
		onNew:   func(s ui.Streams) { got = s },
	}

	require.Contains(t, pressUntil(t, acting(w), 110, 24, []string{"n"}, w.did(1, 1)),
		"ISU-newone1 on report/ISU-newone1")

	_, _, filed, reloads := w.count()
	require.Equal(t, 1, filed)
	require.Positive(t, reloads, "a new issue changes the board")
	require.NotNil(t, got.Out, "the editor was handed somewhere to draw")
	require.NotNil(t, got.In, "and somewhere to read the keyboard")
}

// Never fails silently: every action's failure reaches the screen, in the
// action's own words.
func TestNoActionFailsSilently(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wired   *wired
		reloads int
		want    string
	}{
		{
			"claim", "c", &wired{claimErr: errors.New("the claim did not go")}, 0,
			"the claim did not go",
		},
		{
			"branch", "g", &wired{gotoErr: errors.New("the checkout did not go")}, 0,
			"the checkout did not go",
		},
		{
			"new", "n", &wired{newErr: errors.New("the editor did not go")}, 0,
			"the editor did not go",
		},
		{
			"reload", "c",
			&wired{claimSaid: "done", reloadErr: errors.New("the re-read did not go")}, 1,
			"the re-read did not go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Contains(t, pressUntil(t, acting(tt.wired), 140, 24,
				[]string{tt.key}, tt.wired.did(1, tt.reloads)), tt.want)
		})
	}
}

// An interface with nothing wired to it is a read-only one. Pressing an action
// key there says so rather than doing nothing at all, which is the difference
// between a tool that is read-only and one that is broken.
func TestAnInterfaceWithNoActionsSaysWhyNothingHappened(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 110, 24)

	for _, name := range []string{"c", "g", "n"} {
		require.Contains(t, keys(t, m, name).View(), "read-only",
			"%q on an interface with no actions", name)
	}
}

// An action on an empty board has no issue to act on, and says so rather than
// claiming the empty string.
func TestAnActionWithNothingSelectedSaysSo(t *testing.T) {
	t.Parallel()

	w := &wired{}
	in := input()
	in.Actions = w

	require.Contains(t, keys(t, sized(t, in, 110, 24), "c").View(), "nothing selected")

	claims, _, _, _ := w.count()
	require.Zero(t, claims, "there was nothing to claim")
}

// A reload replaces the derivation and nothing about the terminal, so the
// filter, the fold and the cursor all survive it.
func TestAReloadKeepsWhereSomebodyWas(t *testing.T) {
	t.Parallel()

	w := &wired{claimSaid: "claimed"}
	in := acting(w)

	frame := press(t, in, 110, 24, []string{"j", "j", "c"}, "claimed")

	require.Equal(t, "ISU-triage", selectedIn(frame),
		"the cursor is where it was, on the board that was read again")
}

// M6-S5: "proven by a test that fails if the TUI package calls git
// directly."
//
// internal/gitx's own suite greps internal/ for the call that starts a process,
// which is the rule that nothing outside it builds a git command — and that
// grep is why this comment does not spell the call out. This is the stronger
// statement M6-S5 asks for, about this package specifically: it does not reach
// the git binary, and it does not reach the two packages that do. A renderer
// that could run a git process would run one per frame.
func TestThisPackageCannotCallGit(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"os/exec",
		"github.com/dgorshkov/isu/internal/gitx",
		"github.com/dgorshkov/isu/internal/repo",
		"github.com/dgorshkov/isu/internal/config",
	}

	found, err := filepath.Glob("*.go")
	require.NoError(t, err)
	require.NotEmpty(t, found)

	for _, path := range found {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		require.NoError(t, err)

		for _, imported := range file.Imports {
			name := strings.Trim(imported.Path.Value, `"`)

			require.NotContainsf(t, forbidden, name,
				"%s imports %s: internal/ui is handed everything it draws, and "+
					"everything it cannot do itself goes out through Actions", path, name)
		}
	}
}

// The interface holds a *model.Board and nothing that could load one, which is
// the other half of the same rule: what it reads is a derivation somebody else
// made.
func TestTheInterfaceIsHandedItsDerivation(t *testing.T) {
	t.Parallel()

	in := board()

	require.NotNil(t, in.Board)
	require.IsType(t, &model.Board{}, in.Board)
}
