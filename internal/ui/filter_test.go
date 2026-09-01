package ui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/ui"
)

// M6-S3 · Filter and navigation.

// typed opens the filter and types a needle into it, which is what `/` is for.
func typed(t *testing.T, m ui.Model, needle string) ui.Model {
	t.Helper()

	m = keys(t, m, "/")

	for _, r := range needle {
		m = keys(t, m, string(r))
	}

	return m
}

func TestTheFilterNarrowsAcrossIdTitleTypeStatusAndOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		field  string
		needle string
		want   []string
	}{
		{"id", "openly", []string{"ISU-openly"}},
		{"title", "linter", []string{"ISU-reopen"}},
		{"type", "spike", []string{"ISU-donede"}},
		{"status", "awaiting", []string{"ISU-triage"}},
		{"owner", "support", []string{"ISU-triage"}},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()

			frame := typed(t, sized(t, board(), 100, 24), tt.needle).View()

			require.Equal(t, tt.want, drawnIDs(frame), "filtering on %s", tt.field)
		})
	}
}

// The filter is a substring and not a pattern. Somebody looking for an issue
// about `.*` should find that issue, and somebody who typed a bracket by
// mistake should get an empty list rather than an error nobody can read.
func TestAFilterIsLiteralAndNeverAPattern(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 100, 24)

	for _, needle := range []string{".*", "^ISU", "ISU-.....", "[a-z]+", "(", "a|b"} {
		require.Emptyf(t, drawnIDs(typed(t, m, needle).View()),
			"%q matched something, so it was read as a pattern", needle)
	}

	require.Len(t, drawnIDs(typed(t, m, "ISU-").View()), 7,
		"the literal every id starts with matches every issue")
}

// A filter is a way to find something, so it must not be a way to lose your
// place. The issue somebody was on comes back when the filter that hid it goes.
func TestNarrowingAndClearingRestoresTheSelection(t *testing.T) {
	t.Parallel()

	m := keys(t, sized(t, board(), 100, 24), "down", "down")
	was := selectedIn(m.View())
	require.NotEmpty(t, was)

	narrowed := typed(t, m, "linter")
	require.Equal(t, "ISU-reopen", selectedIn(narrowed.View()))
	require.NotEqual(t, was, selectedIn(narrowed.View()), "the fixture must actually move it")

	require.Equal(t, was, selectedIn(keys(t, narrowed, "esc").View()),
		"clearing the filter puts somebody back where they were")
}

// Where the selected issue still matches, the cursor does not move at all.
func TestTheSelectionSurvivesAFilterItStillMatches(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 100, 24)

	for range 10 {
		if selectedIn(m.View()) == "ISU-reopen" {
			break
		}

		m = keys(t, m, "j")
	}

	require.Equal(t, "ISU-reopen", selectedIn(m.View()))
	require.Equal(t, "ISU-reopen", selectedIn(typed(t, m, "linter").View()))
}

// Filtering to nothing is a state, not a failure, and it says which needle
// found nothing.
func TestAFilterThatMatchesNothingSaysSoAndComesBack(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 100, 24)
	empty := typed(t, m, "nothingmatchesthis")

	require.Empty(t, drawnIDs(empty.View()))
	require.Contains(t, empty.View(), "nothing matches")
	require.Contains(t, empty.View(), "nothingmatchesthis")
	require.Contains(t, empty.View(), "nothing selected", "there is nothing to show on the right")

	back := keys(t, empty, "esc")
	require.Len(t, drawnIDs(back.View()), 7)
}

// Backspace is how a filter is narrowed and widened again without starting
// over, which is the whole of "incremental".
func TestTheFilterIsIncremental(t *testing.T) {
	t.Parallel()

	m := typed(t, sized(t, board(), 100, 24), "linter")
	require.Len(t, drawnIDs(m.View()), 1)

	require.Len(t, drawnIDs(keys(t, m, "backspace", "backspace", "backspace",
		"backspace", "backspace", "backspace").View()), 7,
		"deleting the whole needle is the same as never having typed it")
}

// While the filter is open every printable key is a character in it. A `q` that
// quit the program half way through typing "queue" would be unusable.
func TestKeysTypedIntoTheFilterAreNotCommands(t *testing.T) {
	t.Parallel()

	m := typed(t, sized(t, board(), 100, 24), "qrn")

	require.Contains(t, m.View(), "qrn", "the keys went into the filter")
	require.NotContains(t, m.View(), "ready (", "`r` did not toggle the queue")
}

func TestVimAndArrowKeysMoveTheCursor(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 100, 24)
	first := selectedIn(m.View())

	down := selectedIn(keys(t, m, "j").View())
	require.NotEqual(t, first, down)
	require.Equal(t, down, selectedIn(keys(t, m, "down").View()), "j is down")

	require.Equal(t, first, selectedIn(keys(t, m, "j", "k").View()), "k is up")
	require.Equal(t, first, selectedIn(keys(t, m, "down", "up").View()))

	require.Equal(t, first, selectedIn(keys(t, m, "j", "j", "home").View()))
	require.NotEqual(t, first, selectedIn(keys(t, m, "end").View()))
}

// The cursor never leaves the list, and never lands on a heading: a heading is
// not a thing anybody can claim.
func TestTheCursorStaysOnIssuesAndInsideTheList(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 100, 24)

	for range 30 {
		m = keys(t, m, "j")
	}

	require.Equal(t, "ISU-openly", selectedIn(m.View()), "the last issue on the board")

	for range 30 {
		m = keys(t, m, "k")
	}

	require.Equal(t, "ISU-donede", selectedIn(m.View()), "the first")
}

// PLAN.md M6-S3 asks for navigating across a collapsed epic, which is the one
// case where the row under the cursor is not the row under it on the board.
func TestNavigatingAcrossACollapsedEpic(t *testing.T) {
	t.Parallel()

	m := sized(t, nested(), 100, 24)

	require.Equal(t, []string{
		"ISU-epictop", "ISU-epicmid", "ISU-deepone", "ISU-loosely", "ISU-orphans",
	}, drawnIDs(m.View()))

	// Fold the epic in the middle. Its own child goes with it, and the list
	// below it does not move otherwise.
	m = keys(t, m, "j")
	require.Equal(t, "ISU-epicmid", selectedIn(m.View()))

	m = keys(t, m, "h")
	require.Equal(t, []string{
		"ISU-epictop", "ISU-epicmid", "ISU-loosely", "ISU-orphans",
	}, drawnIDs(m.View()))

	require.Equal(t, "ISU-loosely", selectedIn(keys(t, m, "j").View()),
		"`j` steps over what the fold is hiding rather than into it")

	require.Equal(t, []string{
		"ISU-epictop", "ISU-epicmid", "ISU-deepone", "ISU-loosely", "ISU-orphans",
	}, drawnIDs(keys(t, m, "l").View()), "`l` unfolds it again")
}

// Folding an epic must not take the cursor off the screen with it.
func TestFoldingTheEpicTheCursorIsInsideMovesTheCursorToIt(t *testing.T) {
	t.Parallel()

	m := keys(t, sized(t, nested(), 100, 24), "j", "j")
	require.Equal(t, "ISU-deepone", selectedIn(m.View()))

	folded := keys(t, m, "h")
	require.Equal(t, "ISU-epicmid", selectedIn(folded.View()),
		"folding from inside an epic leaves the cursor on the epic")
}

func TestGoldenFilteredFrame(t *testing.T) {
	t.Parallel()

	golden(t, "filter/narrowed.txt", typed(t, sized(t, board(), 100, 24), "ali").View())
}

// PLAN.md M6-S3: "filtering a 5,000-issue fixture stays inside one frame
// budget." A filter is applied on a keystroke and the frame it produces is the
// only feedback that the keystroke arrived, so a filter that costs more than a
// frame is a filter that feels broken however fast it finishes.
func TestFilteringFiveThousandIssuesStaysInsideOneFrame(t *testing.T) {
	m := sized(t, thousands(5000), 140, 40)

	// The keystroke that narrows five thousand issues to a handful, and the
	// frame it has to produce. Both are timed: rebuilding the list and drawing
	// it are one keypress as far as anybody watching is concerned.
	started := time.Now()

	narrowed := typed(t, m, "4242")
	frame := narrowed.View()

	took := time.Since(started)

	require.Equal(t, []string{"ISU-c04242"}, drawnIDs(frame))
	withinFrameBudget(t, "filtering 5,000 issues", took)
}

// frameBudget is one frame at sixty a second, which is what a terminal redraws
// at and therefore what a keystroke has to fit inside.
const frameBudget = 16 * time.Millisecond

// coverFactor is what the budget above is multiplied by in a binary built for
// coverage, for the reason internal/repo's own gate gives: `make cover` puts a
// counter in every basic block, and what is being measured here is a fold over
// five thousand issues.
const coverFactor = 4

func withinFrameBudget(t *testing.T, what string, took time.Duration) {
	t.Helper()

	held, why := frameBudget, "an uninstrumented binary"
	if testing.CoverMode() != "" {
		held, why = frameBudget*coverFactor, "a binary instrumented for coverage"
	}

	require.Less(t, took, held,
		"%s took %s, and the budget for %s is %s", what, took, why, held)
}

// thousands is a board of n issues under one epic, which is the shape a
// converted backlog has.
func thousands(n int) ui.Input {
	items := make([]*model.Item, 0, n+1)
	children := make([]string, 0, n)

	for i := range n {
		children = append(children, fmt.Sprintf("ISU-c%05d", i))
	}

	items = append(items, item("ISU-epical", "The whole backlog", epic(children...)))

	for i, id := range children {
		items = append(items, item(id, fmt.Sprintf("Imported issue %d", i),
			parent("ISU-epical"), kind(issue.Types[i%len(issue.Types)-1+1])))
	}

	return input(items...)
}

// The filter line is where somebody sees what they have typed, and the footer
// is where it goes.
func TestTheFilterLineShowsWhatWasTyped(t *testing.T) {
	t.Parallel()

	frame := typed(t, sized(t, board(), 100, 24), "ali").View()

	require.Contains(t, frame, "/ali")
	require.True(t, strings.Contains(frame, "esc"), "the way out is on the screen")
}
