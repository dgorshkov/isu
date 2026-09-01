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

	require.Less(t, at(frame, "ISU-epictop"), at(frame, "ISU-epicmid"),
		"an epic comes before the children it folds over")
	require.Less(t, at(frame, "ISU-epicmid"), at(frame, "ISU-deepone"))
	require.Less(t, at(frame, "ISU-deepone"), at(frame, "ISU-loosely"),
		"a nested epic takes its own children with it")

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
	frame := sized(t, in, 100, 40).View()

	for _, group := range in.Groups {
		for _, drawn := range group.Items {
			require.Equalf(t, 1, strings.Count(frame, drawn.ID),
				"%s is drawn %d times", drawn.ID, strings.Count(frame, drawn.ID))
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

	frame := sized(t, input(first, second), 100, 24).View()

	require.Contains(t, frame, "ISU-aaaaaa")
	require.Contains(t, frame, "ISU-bbbbbb")
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

// PLAN.md M6-S2 asks for the empty state as a golden frame, because a blank
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

	drawn := 0

	for _, line := range lines(frame) {
		if strings.Contains(line, "ISU-") && !strings.Contains(line, "│ ISU-") {
			drawn++
		}
	}

	require.Equal(t, 5, drawn, "five issues on the board and five rows in the list")
	require.Contains(t, frame, "open 5")
	require.Contains(t, frame, "5 issues")
}

// at is where an id first appears in a frame, so that a test can say one row
// comes before another without pinning either to a line number.
func at(frame, id string) int { return strings.Index(frame, id) }

// indents is how far each id is drawn from the left, past the cursor marker.
func indents(frame string, ids ...string) []int {
	out := make([]int, 0, len(ids))

	for _, id := range ids {
		for _, line := range lines(frame) {
			cut := strings.Index(line, id)
			if cut < 0 || strings.Contains(line[:cut], "│") {
				continue
			}

			out = append(out, cut-len([]rune("  ")))

			break
		}
	}

	return out
}
