package ui_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/ui"
)

// M6-S4 · Detail pane.
//
// "Done when the detail pane answers 'can I start this?' without leaving the
// TUI", which is a question with four parts: what is it, has anybody got it,
// what is it waiting on, and what does done look like. Every assertion below is
// one of those four.

// folder is a fake Actions holding one issue's attachments and comments. It is
// injected rather than loaded, because loading is the CLI's half of this and
// this package is not allowed to do it.
type folder struct {
	held ui.Folder
	err  error
}

func (f folder) Folder(string) (ui.Folder, error) { return f.held, f.err }

// drive runs the interface through the real event loop, which is the only way
// a command it returns actually runs — the folder is fetched by one.
func drive(t *testing.T, in ui.Input, width, height int, until string, then ...string) string {
	t.Helper()

	tm := teatest.NewTestModel(t, ui.New(in), teatest.WithInitialTermSize(width, height))

	if until != "" {
		teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
			return bytes.Contains(out, []byte(until))
		}, teatest.WithDuration(5*time.Second))
	}

	for _, name := range then {
		tm.Send(key(name))
	}

	tm.Send(key("q"))
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	final, ok := tm.FinalModel(t).(ui.Model)
	require.True(t, ok)

	return final.View()
}

// detailPane is the right-hand half of a frame.
func detailPane(frame string) string {
	var out []string

	for _, line := range lines(frame) {
		if cut := strings.Index(line, gutter); cut >= 0 {
			out = append(out, strings.TrimPrefix(line[cut:], gutter))
		}
	}

	return strings.Join(out, "\n")
}

// PLAN.md M6-S4 asks for a golden frame per type, because what a type requires
// before it can be closed is the one thing the pane must never leave out.
func TestGoldenDetailForEveryType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   ui.Input
	}{
		{"bug", oneIssue(item("ISU-abug01", "Login retries drop the second attempt",
			priority(issue.PriorityP1),
			field(func(i *issue.Issue) {
				i.Repro = "post the form twice inside a second"
				i.Body = "# What happens\n\nThe second POST is dropped, and nothing says so.\n"
			})))},
		{"story", oneIssue(item("ISU-astory", "The board renders epics",
			kind(issue.TypeStory),
			field(func(i *issue.Issue) {
				i.Acceptance = "an epic shows its children indented beneath it"
			})))},
		{"chore", oneIssue(item("ISU-achore", "Upgrade the linter", kind(issue.TypeChore)))},
		{"spike", oneIssue(item("ISU-aspike", "What does contention cost?",
			kind(issue.TypeSpike),
			field(func(i *issue.Issue) { i.Question = "how often do two clones race?" })))},
		{"epic", nested()},
		{"dropped", oneIssue(item("ISU-adrops", "Rewrite it in another language",
			kind(issue.TypeChore), status(model.StatusDropped), state(issue.StateDropped),
			field(func(i *issue.Issue) {
				i.Reason = "we like this one"
				i.Resolution = issue.ResolutionWontfix
			})))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			golden(t, "detail/"+tt.name+".txt", sized(t, tt.in, 100, 24).View())
		})
	}
}

// oneIssue is a board holding one thing, for a frame that is about the pane and
// not about the list.
func oneIssue(it *model.Item) ui.Input { return input(it) }

// The body is markdown and is rendered as markdown. `isu show` prints it
// verbatim and says so; this is the pane that does not have to.
func TestTheBodyIsRenderedAsMarkdown(t *testing.T) {
	t.Parallel()

	in := oneIssue(item("ISU-abody1", "Something happened",
		field(func(i *issue.Issue) {
			i.Body = "# A heading\n\n- one\n- two\n\n`code`\n"
		})))

	pane := detailPane(sized(t, in, 100, 30).View())

	require.Contains(t, pane, "A heading")
	require.Contains(t, pane, "one")
	require.NotContains(t, pane, "- one", "a list marker is rendered, not printed")
}

// An issue that waits on something says what it waits on and whether that has
// finished, which is most of "can I start this?".
func TestABlockerIsShownWithItsOwnStatus(t *testing.T) {
	t.Parallel()

	blocker := item("ISU-blocker", "The thing underneath", status(model.StatusInProgress))
	blocked := item("ISU-blocked", "The thing on top",
		field(func(i *issue.Issue) { i.BlockedBy = []string{"ISU-blocker", "ISU-nobody"} }))

	pane := detailPane(keys(t, sized(t, input(blocker, blocked), 140, 30), "j").View())

	require.Contains(t, pane, "ISU-blocked", "the fixture selected the wrong issue")
	require.Contains(t, pane, "ISU-blocker")
	require.Contains(t, pane, "in progress", "a blocker that has not finished says so")
	require.Contains(t, pane, "no issue with this id",
		"a blocker naming nothing is reported, not dropped")
}

// An epic's pane is its children and where they have got to; a child's pane is
// which epic it belongs to and where in it it sits.
func TestAnEpicShowsItsChildrenAndAChildShowsItsPlace(t *testing.T) {
	t.Parallel()

	m := sized(t, nested(), 110, 30)

	epicPane := detailPane(m.View())
	require.Contains(t, epicPane, "children")
	require.Contains(t, epicPane, "ISU-epicmid")
	require.Contains(t, epicPane, "ISU-loosely")

	childPane := detailPane(keys(t, m, "j", "j").View())
	require.Contains(t, childPane, "ISU-deepone", "the fixture selected the wrong issue")
	require.Contains(t, childPane, "ISU-epicmid", "a child names the epic it belongs to")
	require.Contains(t, childPane, "1 of 1", "and where in it it sits")
}

// Two branches claiming one issue is the case this whole product is about, and
// the pane names both of them: "somebody has this" and "two people have this"
// are different answers to "can I start this?".
func TestAContendedIssueShowsBothClaimants(t *testing.T) {
	t.Parallel()

	contended := item("ISU-fought", "Everybody wants this one",
		status(model.StatusInProgress),
		claimedBy("alice", "isu/ISU-fought", 3),
		claimedBy("bob", "isu/ISU-fought-2", 1))

	pane := detailPane(sized(t, oneIssue(contended), 110, 30).View())

	require.Contains(t, pane, "contended")
	require.Contains(t, pane, "alice")
	require.Contains(t, pane, "bob")
	require.Contains(t, pane, "isu/ISU-fought")
	require.Contains(t, pane, "isu/ISU-fought-2")
}

// What lives beside an issue is loaded, not derived, so it arrives through the
// injected action and the pane draws it when it does.
func TestAttachmentsAndCommentsAreShownWhenTheyArrive(t *testing.T) {
	t.Parallel()

	in := oneIssue(item("ISU-hasbits", "An issue with things beside it"))
	in.Actions = folder{held: ui.Folder{
		Attachments: []string{"repro.har", "screenshot.png"},
		Comments: []ui.Comment{
			{Name: "2026-08-24-support-01.md", Body: "It happens on Firefox too.\n"},
		},
	}}

	pane := detailPane(drive(t, in, 110, 30, "repro.har"))

	require.Contains(t, pane, "attachments")
	require.Contains(t, pane, "repro.har")
	require.Contains(t, pane, "screenshot.png")
	require.Contains(t, pane, "2026-08-24-support-01.md")
	require.Contains(t, pane, "It happens on Firefox too.")
}

// Never fails silently, which is M6-S5's rule and applies to the read as much
// as to the writes: a folder that could not be read says so rather than
// rendering as an issue with nothing beside it.
func TestAFolderThatCannotBeReadSaysSo(t *testing.T) {
	t.Parallel()

	in := oneIssue(item("ISU-unreadb", "Something beside it will not open"))
	in.Actions = folder{err: errors.New("permission denied")}

	require.Contains(t, detailPane(drive(t, in, 110, 30, "permission denied")),
		"permission denied")
}

// PLAN.md M6-S4: "an issue with twenty attachments scrolls".
func TestAnIssueWithTwentyAttachmentsScrolls(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, 20)
	for i := range 20 {
		names = append(names, fmt.Sprintf("capture-%02d.har", i))
	}

	in := oneIssue(item("ISU-manybit", "Twenty things beside it"))
	in.Actions = folder{held: ui.Folder{Attachments: names}}

	before := detailPane(drive(t, in, 100, 24, names[0]))
	require.Contains(t, before, names[0])
	require.NotContains(t, before, names[19], "the pane is not tall enough for all twenty")

	scrolled := detailPane(drive(t, in, 100, 24, names[0], "enter", "end"))

	require.Contains(t, scrolled, names[19], "the pane scrolled to the end of them")
	require.NotContains(t, scrolled, names[0], "and away from the start")

	// And one line at a time gets there too, which is what `j` is for.
	stepped := detailPane(drive(t, in, 100, 24, names[0], "enter",
		"j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j", "j"))
	require.Contains(t, stepped, names[19])

	// Scrolling back is scrolling back, and stops at the top.
	require.Contains(t, detailPane(drive(t, in, 100, 24, names[0],
		"enter", "end", "home")), names[0])
	require.Contains(t, detailPane(drive(t, in, 100, 24, names[0],
		"enter", "end", "pgup", "pgup", "pgup")), names[0])
	require.Contains(t, detailPane(drive(t, in, 100, 24, names[0],
		"enter", "pgdown", "k", "k")), names[10])
}

// `enter` moves the keys to the pane and `esc` gives them back, which is the
// difference between scrolling a body and walking off the end of the list.
func TestEnterFocusesThePaneAndEscapeReturnsToTheList(t *testing.T) {
	t.Parallel()

	m := sized(t, board(), 100, 24)
	first := selectedIn(m.View())

	focused := keys(t, m, "enter", "j", "j")
	require.Equal(t, first, selectedIn(focused.View()),
		"`j` scrolls the pane while it has the keys, and does not move the list")
	require.Contains(t, focused.View(), "esc", "the way back is on the screen")

	require.NotEqual(t, first, selectedIn(keys(t, focused, "esc", "j").View()),
		"`esc` gives the keys back to the list")
}

// A pane with nothing left to scroll to does not scroll, and a body shorter
// than the pane never moves at all.
func TestAPaneWithNothingBelowItDoesNotScroll(t *testing.T) {
	t.Parallel()

	m := keys(t, sized(t, board(), 100, 40), "enter")
	before := m.View()

	require.Equal(t, before, keys(t, m, "j", "j", "j", "j").View())
}

// The interface still opens when nothing is wired to it, which is what every
// test above that does not name an action relies on.
func TestAnInterfaceWithNoActionsStillDrawsThePane(t *testing.T) {
	t.Parallel()

	in := oneIssue(item("ISU-nowired", "Nothing is wired up"))
	in.Actions = nil

	require.Contains(t, detailPane(sized(t, in, 100, 24).View()), "ISU-nowired")
}

// A terminal too narrow to wrap markdown into gets the body as it was written,
// which is what `isu show` does anyway.
func TestAVeryNarrowPaneShowsTheBodyVerbatim(t *testing.T) {
	t.Parallel()

	in := oneIssue(item("ISU-narrow1", "Narrow",
		field(func(i *issue.Issue) { i.Body = "# Heading\n" })))

	require.Contains(t, sized(t, in, 30, 24).View(), "# Heading")
}

// A resize re-wraps the body, because the wrap width is the pane's.
func TestResizingRewrapsTheBody(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("word ", 40)
	in := oneIssue(item("ISU-wrapsit", "Wraps", field(func(i *issue.Issue) { i.Body = long })))

	narrow := sized(t, in, 100, 40)
	wide := send(t, narrow, tea.WindowSizeMsg{Width: 200, Height: 40})

	require.NotEqual(t, detailPane(narrow.View()), detailPane(wide.View()))
}
