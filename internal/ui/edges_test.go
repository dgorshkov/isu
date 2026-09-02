package ui_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
	"github.com/dgorshkov/isu/internal/ui"
)

// The cases a board grows on its way to being real: an issue whose file will
// not decode, an epic pointing at something that is not there, a claim nobody
// can name, a terminal the size of a postage stamp.
//
// Every one of these is a row this interface still has to draw. A renderer that
// only handles the fixture is a renderer that meets a repository once.

// broken is trunk carrying a folder whose file will not decode, which is the
// row the loader keeps deliberately so that a half-written issue does not blind
// the whole board.
func broken(id string) *model.Item {
	return &model.Item{
		ID: id, OnTrunk: true, Status: model.StatusOpen,
		Broken: &repo.Broken{
			ID: id, Path: "issues/" + id + "/README.md",
			Err: errors.New("line 1: the file must open with ---"),
		},
	}
}

func TestAnIssueNothingCanDecodeStillHasARow(t *testing.T) {
	t.Parallel()

	frame := sized(t, input(broken("ISU-brokens")), 100, 24)

	require.Equal(t, []string{"ISU-brokens"}, drawnIDs(frame.View()))
	require.Contains(t, frame.View(), "(no title)")
	require.Contains(t, frame.View(), "unreadable")
	require.Contains(t, detailPane(frame.View()), "must open with")

	// And the filter finds it by the only two things it has: its id and the
	// status the table gave it.
	require.Len(t, drawnIDs(typed(t, frame, "brokens").View()), 1)
	require.Empty(t, drawnIDs(typed(t, frame, "chore").View()),
		"a file nothing can decode has no type to match on")
}

// Contended, stale and reopened all survive the status they lost to, and the
// row says so.
func TestTheAnnotationsThatSurviveTheStatusAreOnTheRow(t *testing.T) {
	t.Parallel()

	it := item("ISU-noteful", "Everything at once", status(model.StatusInProgress),
		reopened(), claimedBy("alice", "isu/ISU-noteful", 40),
		claimedBy("", "isu/ISU-noteful-2", 1))
	it.Claims[0].Stale = true

	pane := detailPane(sized(t, input(it), 120, 24).View())

	require.Contains(t, pane, "in progress (reopened, contended, stale)")
	require.Contains(t, pane, "claimed by alice on isu/ISU-noteful — stale")
	require.Contains(t, pane, "claimed by someone on isu/ISU-noteful-2",
		"a claim whose first commit was not looked up is still a claim")
}

// An epic naming a child the board does not hold is `isu check`'s to report,
// and the pane says what it is rather than dropping the row.
func TestAnEpicNamingAChildThatIsNotThereSaysSo(t *testing.T) {
	t.Parallel()

	pane := detailPane(sized(t,
		input(item("ISU-epicbad", "Points at nothing", epic("ISU-nobody1"))), 100, 24).View())

	require.Contains(t, pane, "ISU-nobody1")
	require.Contains(t, pane, "no issue with this id")
}

func TestAnEpicWithNoChildrenSaysItFoldsOverNothing(t *testing.T) {
	t.Parallel()

	require.Contains(t, detailPane(sized(t,
		input(item("ISU-epicnil", "Nobody filled it in", epic())), 100, 24).View()),
		"folds over nothing")
}

// An issue naming an epic it is not among the children of has no position in
// it, and the pane simply does not claim one.
func TestAChildTheEpicDoesNotListHasNoPosition(t *testing.T) {
	t.Parallel()

	pane := detailPane(keys(t, sized(t, input(
		item("ISU-epicish", "An epic", epic("ISU-someone")),
		item("ISU-strayer", "Says it belongs", parent("ISU-epicish")),
	), 120, 24), "j").View())

	require.Contains(t, pane, "ISU-strayer")
	require.NotContains(t, pane, "position")
}

// An interface handed no board can draw every issue it was given; what it
// cannot do is say what an id points at.
func TestWithoutABoardALinkIsJustAnId(t *testing.T) {
	t.Parallel()

	in := input(item("ISU-linkers", "Waits on something",
		field(func(i *issue.Issue) { i.BlockedBy = []string{"ISU-blocker"} })))
	in.Board = nil

	require.Contains(t, detailPane(sized(t, in, 120, 24).View()),
		"ISU-blocker  (no issue with this id)")
}

// An issue with no date does not print one, rather than printing the zero year.
func TestAnIssueWithNoDatePrintsNoDate(t *testing.T) {
	t.Parallel()

	undated := item("ISU-undated", "Filed by an importer")
	undated.Issue.Created = time.Time{}

	require.NotContains(t, detailPane(sized(t, input(undated), 100, 24).View()), "0001")
}

// A terminal too small to be useful is still one this interface must not crash
// on, and every line of the frame is still inside it.
func TestATerminalTheSizeOfAPostageStamp(t *testing.T) {
	t.Parallel()

	m := ui.New(board())

	for _, size := range [][2]int{{1, 1}, {2, 3}, {4, 8}, {20, 6}, {80, 24}} {
		m = send(t, m, resize(size[0], size[1]))

		frame := lines(m.View())

		require.Lenf(t, frame, size[1], "%dx%d", size[0], size[1])

		for _, line := range frame {
			require.LessOrEqualf(t, len([]rune(line)), size[0],
				"%dx%d: %q", size[0], size[1], line)
		}
	}
}

// `r` on a repository where nothing is ready says why, rather than showing an
// empty pane.
func TestAnEmptyReadyQueueSaysWhyItIsEmpty(t *testing.T) {
	t.Parallel()

	require.Contains(t, keys(t, sized(t, board(), 100, 24), "r").View(),
		"nothing is ready")
}

// The filter line stays on the screen after `enter` closes it, because the list
// is still narrowed and a list that is narrowed without saying so is a list
// somebody thinks has lost their issues.
func TestTheFilterSaysItIsStillOnAfterItsLineCloses(t *testing.T) {
	t.Parallel()

	m := keys(t, typed(t, sized(t, board(), 100, 24), "linter"), "enter")

	require.Len(t, drawnIDs(m.View()), 1)
	require.Contains(t, m.View(), "/linter")
	require.Contains(t, m.View(), "esc clears it")
	require.Contains(t, m.View(), "enter open", "the keys are the list's again")
}

// The arrow keys still move the list while the filter line is open, which is
// what the hints on that line offer: narrowing to four issues and then picking
// one of them should not need the line closed first.
func TestTheArrowsStillMoveWhileTheFilterLineIsOpen(t *testing.T) {
	t.Parallel()

	m := typed(t, sized(t, board(), 100, 24), "isu-")

	require.Len(t, drawnIDs(m.View()), 7)
	require.Equal(t, "ISU-donede", selectedIn(m.View()))
	require.NotEqual(t, "ISU-donede", selectedIn(keys(t, m, "down").View()),
		"the filter line takes the printable keys and leaves the arrows")
	require.Contains(t, keys(t, m, "down").View(), "/isu-", "and the needle is still being typed")
}

// `esc` clears a filter from the list as well as from the line, because a list
// that is still narrowed after somebody has stopped typing needs a way out that
// is not retyping the needle to delete it.
func TestEscapeClearsAFilterWhoseLineIsAlreadyClosed(t *testing.T) {
	t.Parallel()

	closed := keys(t, typed(t, sized(t, board(), 100, 24), "linter"), "enter")
	require.Len(t, drawnIDs(closed.View()), 1)

	require.Len(t, drawnIDs(keys(t, closed, "esc").View()), 7)
}

// An action that did what it was asked and had nothing to say still says
// something. Never failing silently is not only about failures.
func TestAnActionWithNothingToReportStillReports(t *testing.T) {
	t.Parallel()

	w := &wired{}

	require.Contains(t, pressUntil(t, acting(w), 110, 24, []string{"c"}, w.did(1, 1)),
		"claim: nothing to do")
}

// Folding is only ever about an epic. On an issue that is in none, `h` is
// nothing at all rather than a fold of something else.
func TestFoldingSomethingThatIsInNoEpicDoesNothing(t *testing.T) {
	t.Parallel()

	m := sized(t, input(item("ISU-alonely", "In no epic at all")), 100, 24)

	require.Equal(t, m.View(), keys(t, m, "h").View())
	require.Equal(t, m.View(), keys(t, m, "l").View())
}

// And on an empty board there is nothing to fold, claim or scroll to.
func TestAnEmptyBoardAnswersEveryKeyWithoutMoving(t *testing.T) {
	t.Parallel()

	m := sized(t, input(), 100, 24)

	for _, name := range []string{"h", "l", "j", "k", "home", "end", "pgdown", "pgup"} {
		require.Equalf(t, m.View(), keys(t, m, name).View(), "%q moved an empty board", name)
	}
}

// The list scrolls, and stops. A cursor that walked off the bottom would take
// the whole list with it.
func TestTheListScrollsAndStopsAtTheEnd(t *testing.T) {
	t.Parallel()

	m := sized(t, fortyChildren(), 100, 24)

	require.Equal(t, "ISU-epical", selectedIn(m.View()))

	bottom := keys(t, m, "end")
	require.Equal(t, "ISU-c00039", selectedIn(bottom.View()))
	require.Equal(t, bottom.View(), keys(t, bottom, "pgdown", "j", "end").View())

	top := keys(t, bottom, "home")
	require.Equal(t, "ISU-epical", selectedIn(top.View()))
	require.Equal(t, top.View(), keys(t, top, "pgup", "k", "home").View())

	// And a page is a page rather than the whole list.
	paged := keys(t, m, "pgdown")
	require.NotEqual(t, selectedIn(m.View()), selectedIn(paged.View()))
	require.NotEqual(t, selectedIn(bottom.View()), selectedIn(paged.View()))
}

// One issue can be on the board and in the ready queue at once, and it is
// indexed for the filter once either way.
func TestAnIssueInBothTheBoardAndTheQueueIsIndexedOnce(t *testing.T) {
	t.Parallel()

	in := board()
	in.Ready = []*model.Item{in.Groups[len(in.Groups)-1].Items[1]}

	ready := typed(t, keys(t, sized(t, in, 100, 24), "r"), "openly")

	require.Equal(t, []string{"ISU-openly"}, drawnIDs(ready.View()))
}

// A filter that hides everything hides the pane too, and typing more into it
// does not resurrect a selection.
func TestAFilterThatMatchesNothingLeavesNothingSelected(t *testing.T) {
	t.Parallel()

	// `enter` closes the filter line and keeps the needle, which is what puts
	// the keys back in the list's hands.
	m := keys(t, typed(t, sized(t, board(), 100, 24), "zzz"), "enter")

	require.Contains(t, m.View(), "nothing selected")
	require.Contains(t, keys(t, m, "c").View(), "nothing selected",
		"an action with nothing selected says so rather than acting on nothing")
	require.Equal(t, m.View(), keys(t, m, "h", "l", "j", "k").View())
}

// A body that is only whitespace is no body, and the pane does not open a
// blank markdown block for it.
func TestABodyOfNothingIsNotDrawn(t *testing.T) {
	t.Parallel()

	blank := item("ISU-blankbd", "Nothing below the frontmatter",
		field(func(i *issue.Issue) { i.Body = "\n  \n" }))

	require.Equal(t,
		detailPane(sized(t, input(item("ISU-blankbd", "Nothing below the frontmatter")),
			100, 24).View()),
		detailPane(sized(t, input(blank), 100, 24).View()))
}

// An attachment list that arrives empty is not a heading with nothing under it.
func TestAFolderWithNothingInItAddsNothingToThePane(t *testing.T) {
	t.Parallel()

	in := input(item("ISU-emptybe", "Nothing beside it"))
	in.Actions = &wired{}

	require.NotContains(t, drive(t, in, 100, 24, "ISU-emptybe"), "attachments")
}

// Comments are wrapped like everything else in the pane, so a paragraph
// somebody pasted does not run off the side of it.
func TestALongCommentIsWrappedIntoThePane(t *testing.T) {
	t.Parallel()

	in := input(item("ISU-wordycm", "Somebody said a lot"))
	in.Actions = &wired{held: ui.Folder{Comments: []ui.Comment{
		{Name: "2026-09-01-alice-01.md", Body: strings.Repeat("word ", 60)},
	}}}

	pane := detailPane(drive(t, in, 100, 30, "alice-01"))

	require.Contains(t, pane, "comment 2026-09-01-alice-01.md")
	require.GreaterOrEqual(t, strings.Count(pane, "word word"), 2, "it wrapped")
}
