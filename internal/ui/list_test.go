package ui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/ui"
)

// M6-S2 · List and grouping.
//
// The grouping is `isu board`'s, and it arrives already grouped — that half is
// asserted where both renderers can be run against one repository, in
// internal/cli. What is asserted here is what this package does with it: an
// epic's children are drawn under it, and every issue is drawn exactly once
// however deep the nesting goes.

// nested is an epic holding an epic, which is the shape a milestone made of
// stories has and the one an arrangement gets wrong.
func nested() ui.Input {
	top := item("ISU-epictop", "Ship the tracker", epic("ISU-epicmid", "ISU-loosely"))
	mid := item("ISU-epicmid", "Make login reliable", epic("ISU-deepone"),
		parent("ISU-epictop"))
	deep := item("ISU-deepone", "Retries drop the second attempt",
		parent("ISU-epicmid"), priority(issue.PriorityP1))
	loose := item("ISU-loosely", "Rotate the signing key", parent("ISU-epictop"),
		kind(issue.TypeChore))
	orphan := item("ISU-orphans", "Nobody's child")

	return input(top, mid, deep, loose, orphan)
}

func TestAnEpicsChildrenAreDrawnUnderIt(t *testing.T) {
	t.Parallel()

	frame := sized(t, nested(), 100, 24).View()

	golden(t, "list/nested.txt", frame)

	require.Equal(t,
		[]string{"ISU-epictop", "ISU-epicmid", "ISU-deepone", "ISU-loosely", "ISU-orphans"},
		drawnIDs(frame),
		"an epic comes before the children it folds over, and a nested epic takes "+
			"its own children with it")

	require.Equal(t, []int{0, 2, 4, 2, 0}, indents(frame,
		"ISU-epictop", "ISU-epicmid", "ISU-deepone", "ISU-loosely", "ISU-orphans"),
		"depth is how far an issue is from the top of its epic")
}

// Nesting rearranges a group; it must never lose from one or add to one. The
// board is the set of issues, and a list that quietly drops the one whose
// parent it could not place is a list that hides exactly the issue somebody
// most needs to see.
func TestEveryIssueIsDrawnExactlyOnceWhateverTheNesting(t *testing.T) {
	t.Parallel()

	in := nested()
	list := strings.Join(listPane(sized(t, in, 100, 40).View()), "\n")

	for _, group := range in.Groups {
		for _, drawn := range group.Items {
			require.Equalf(t, 1, strings.Count(list, drawn.ID),
				"%s is drawn %d times", drawn.ID, strings.Count(list, drawn.ID))
		}
	}
}

// Two epics that are each other's parents are `isu check`'s to report, and the
// list still has to draw both of them. An arrangement that followed the parent
// chain without a guard would follow this one until the stack ran out.
func TestAParentCycleStillDrawsBothOfItsEpics(t *testing.T) {
	t.Parallel()

	first := item("ISU-aaaaaa", "The first half", epic("ISU-bbbbbb"), parent("ISU-bbbbbb"))
	second := item("ISU-bbbbbb", "The second half", epic("ISU-aaaaaa"), parent("ISU-aaaaaa"))

	require.Equal(t, []string{"ISU-aaaaaa", "ISU-bbbbbb"},
		drawnIDs(sized(t, input(first, second), 100, 24).View()))
}

// Indentation is inside a group, because the groups are the board's and the
// board puts an issue where its status says. A child whose epic is finished
// stands at the top of its own group rather than being drawn under an epic
// that is three groups away.
func TestAChildWhoseEpicIsInAnotherGroupStandsOnItsOwn(t *testing.T) {
	t.Parallel()

	done := item("ISU-epicdun", "The finished epic", epic("ISU-stillon"),
		status(model.StatusDone))
	child := item("ISU-stillon", "Still open under it", parent("ISU-epicdun"))

	frame := sized(t, input(done, child), 100, 24).View()

	require.Equal(t, []int{0, 0}, indents(frame, "ISU-epicdun", "ISU-stillon"))
}

// An epic with forty children is the case that turns a pane into a scroll
// region, and the frame is still exactly the terminal.
func TestAnEpicWithFortyChildren(t *testing.T) {
	t.Parallel()

	golden(t, "list/forty-children.txt", sized(t, fortyChildren(), 100, 24).View())
}

func fortyChildren() ui.Input {
	children := make([]string, 0, 40)
	items := make([]*model.Item, 0, 41)

	for i := range 40 {
		id := fmt.Sprintf("ISU-c%05d", i)
		children = append(children, id)
	}

	items = append(items, item("ISU-epical", "Convert the whole backlog", epic(children...)))

	for i, id := range children {
		items = append(items, item(id, fmt.Sprintf("Import batch %d", i+1), parent("ISU-epical")))
	}

	return input(items...)
}

// M6-S2 asks for the empty state as a golden frame, because a blank
// pane is the one output nobody can tell from a crash.
func TestAListOfZeroIssuesRendersTheEmptyState(t *testing.T) {
	t.Parallel()

	golden(t, "list/empty.txt", sized(t, input(), 80, 24).View())
}

// The counts in the header are the groups under them, added up. Two numbers on
// one screen that disagree is the fastest way to make somebody stop believing
// either.
func TestTheHeaderCountIsTheNumberOfRowsBeneathIt(t *testing.T) {
	t.Parallel()

	in := nested()
	frame := sized(t, in, 100, 40).View()

	require.Len(t, drawnIDs(frame), 5, "five issues on the board and five rows in the list")
	require.Contains(t, frame, "open 5")
	require.Contains(t, frame, "5 issues")
}

// listPane is the left-hand half of a frame, which is the list. The detail pane
// names the selected issue too, so a test counting rows has to look at one pane
// rather than at the screen.
func listPane(frame string) []string {
	var out []string

	for _, line := range lines(frame) {
		if cut := strings.Index(line, gutter); cut >= 0 {
			out = append(out, strings.TrimRight(line[:cut], " "))
		}
	}

	return out
}

// gutter is what separates the two panes.
const gutter = "│"

// drawnIDs is every issue the list drew, in the order it drew them.
func drawnIDs(frame string) []string {
	var out []string

	for _, line := range listPane(frame) {
		if id := strings.TrimLeft(line, " ▸"); strings.HasPrefix(id, "ISU-") {
			out = append(out, strings.Fields(id)[0])
		}
	}

	return out
}

// selectedIn is the id the cursor is on, read off a frame.
func selectedIn(frame string) string {
	for _, line := range listPane(frame) {
		if rest, found := strings.CutPrefix(line, "▸ "); found {
			return strings.Fields(rest)[0]
		}
	}

	return ""
}

// indents is how far each id is drawn from the left, past the cursor marker —
// which is measured in characters, because the marker is not an ASCII one.
func indents(frame string, ids ...string) []int {
	out := make([]int, 0, len(ids))

	for _, id := range ids {
		for _, line := range listPane(frame) {
			runes := []rune(line)

			cut := strings.Index(string(runes), id)
			if cut < 0 {
				continue
			}

			out = append(out, len([]rune(line[:cut]))-markerWidth)

			break
		}
	}

	return out
}

// markerWidth is the cursor column every row starts past.
const markerWidth = 2
