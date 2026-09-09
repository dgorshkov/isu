package ui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/ui"
)

// M6-S1 · Shell, layout and key map.
//
// A frame is this package's whole output, so the golden files are the review:
// two sizes, because a layout that only ever ran at one is a layout that has
// never been resized.

func TestTheShellRendersAtEightyByTwentyFour(t *testing.T) {
	t.Parallel()

	golden(t, "shell/80x24.txt", sized(t, board(), 80, 24).View())
}

func TestTheShellRendersAtOneHundredAndFortyByForty(t *testing.T) {
	t.Parallel()

	golden(t, "shell/140x40.txt", sized(t, board(), 140, 40).View())
}

// A frame is exactly the terminal it was drawn for. A line too many scrolls the
// header off the top of the screen on every redraw, and a line too few leaves
// the previous frame's footer sitting under this one.
func TestEveryFrameIsExactlyTheSizeOfItsTerminal(t *testing.T) {
	t.Parallel()

	sizes := [][2]int{{80, 24}, {140, 40}, {100, 30}, {80, 24}, {200, 60}, {80, 24}}

	m := ui.New(board())

	for _, size := range sizes {
		width, height := size[0], size[1]

		m = send(t, m, resize(width, height))

		frame := lines(m.View())

		require.Lenf(t, frame, height, "%dx%d: wrong number of lines", width, height)

		for i, line := range frame {
			require.LessOrEqualf(t, len([]rune(line)), width,
				"%dx%d: line %d is wider than the terminal: %q", width, height, i, line)
		}
	}
}

// The same size twice is the same frame. A layout that drifts under a resize
// sequence is one that is accumulating state it should be deriving.
func TestAResizeSequenceEndsWhereItStarted(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 80, 24)
	before := m.View()

	m = send(t, m, resize(140, 40), resize(100, 30), resize(80, 24))

	require.Equal(t, before, m.View())
}

// The header counts the issues in the list under it. Two numbers on one screen
// that disagree is the fastest way to make somebody stop believing either.
func TestTheHeaderCountsWhatTheListHolds(t *testing.T) {
	t.Parallel()

	in := board()
	m := sized(t, in, 140, 40)
	head := lines(m.View())[0] + lines(m.View())[1]

	total := 0

	for _, group := range in.Groups {
		total += len(group.Items)

		require.Containsf(t, head, string(group.Status), "the header does not name %s", group.Status)
	}

	require.Contains(t, head, "7 issues")
	require.Equal(t, 7, total, "the fixture is one issue in every status and an epic")
	require.Contains(t, head, "main", "the header says which ref it read as trunk")
	require.Contains(t, head, "remote refs 2h ago")
}

// M6-S1 names the key map, and the footer is where a person learns it.
func TestTheFooterNamesEveryKeyThePlanAsksFor(t *testing.T) {
	t.Parallel()

	frame := sized(t, board(), 140, 40).View()
	footer := lines(frame)[len(lines(frame))-1]

	for _, hint := range []string{"enter", "c", "n", "r", "g", "/", "q"} {
		require.Containsf(t, footer, hint, "the footer does not offer %q", hint)
	}

	for _, word := range []string{"open", "claim", "new", "ready", "branch", "filter", "quit"} {
		require.Containsf(t, footer, word, "the footer does not say what a key does: %q", word)
	}
}

// A key nothing is bound to changes nothing. A TUI that redraws differently
// when somebody leans on the keyboard is a TUI nobody trusts with `c`.
func TestAKeyNothingIsBoundToChangesNothing(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 80, 24)
	before := m.View()

	require.Equal(t, before, keys(t, m, "z", "!", "ctrl+z").View())
}

// `r` swaps the list for the ready queue and back, which is the one thing in
// this key map that changes what the list is about rather than where it is.
func TestTheReadyQueueIsAKeyAway(t *testing.T) {
	t.Parallel()

	in := board()
	in.Ready = []*model.Item{in.Groups[len(in.Groups)-1].Items[1]}

	m := sized(t, in, 80, 24)

	ready := keys(t, m, "r").View()
	require.Contains(t, ready, "ready (1)")
	require.NotContains(t, ready, "ISU-donede", "a finished issue is not ready")

	back := keys(t, m, "r", "r").View()
	require.Contains(t, back, "ISU-donede", "`r` is a toggle: the board comes back")
	require.NotContains(t, back, "ready (1)")
	require.Equal(t, "ISU-openly", selectedIn(back),
		"the cursor followed the issue the queue was showing, which is what makes "+
			"`r` a way to find something rather than a way to lose your place")
}

// The plan: "q quits cleanly and restores the terminal". Quitting cleanly is
// this half — the program ends of its own accord, rather than being torn down
// by a timeout — and restoring the terminal is asserted where the terminal is,
// against `isu ui` itself.
func TestQPressedInTheProgramQuitsIt(t *testing.T) {
	t.Parallel()

	tm := teatest.NewTestModel(t, ui.New(board()), teatest.WithInitialTermSize(80, 24))

	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	final, ok := tm.FinalModel(t).(ui.Model)
	require.True(t, ok)
	require.NotEmpty(t, final.View(), "the last frame is still a frame")
}

// The program is driven end to end here rather than one Update at a time: a
// model that renders correctly under direct calls and deadlocks under the real
// event loop is a model nobody can run.
func TestTheProgramDrawsTheBoardItWasHanded(t *testing.T) {
	t.Parallel()

	tm := teatest.NewTestModel(t, ui.New(board()), teatest.WithInitialTermSize(140, 40))

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return strings.Contains(string(out), "ISU-openly")
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// A board with nothing on it is the state a repository starts in, and it has to
// render as a sentence rather than as a blank pane.
func TestAnEmptyBoardSaysSoRatherThanShowingNothing(t *testing.T) {
	t.Parallel()

	frame := sized(t, input(), 80, 24).View()

	require.Contains(t, frame, "no issues")
	require.Contains(t, frame, "0 issues")
}
