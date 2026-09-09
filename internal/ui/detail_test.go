package ui_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// What lives beside an issue is injected rather than loaded, because loading is
// the CLI's half of this and this package is not allowed to do it. The fake is
// `wired`, in actions_test.go, which stands in for internal/cli as a whole.

// drive runs the interface through the real event loop, which is the only way
// a command it returns actually runs — the folder is fetched by one.
//
// It waits for something on the screen before it types, because scrolling a
// pane that has not been filled yet scrolls nothing.
func drive(t *testing.T, in ui.Input, width, height int, until string, then ...string) string {
	t.Helper()

	return script{width: width, height: height, before: until, keys: then}.run(t, in)
}

// press is the other order: type first, and wait for what the typing produced.
// An action reports after it has run, so there is nothing to wait for until the
// key that starts it has been sent.
func press(t *testing.T, in ui.Input, width, height int, keys []string, until string) string {
	t.Helper()

	return script{width: width, height: height, keys: keys, after: until}.run(t, in)
}

// pressUntil waits on a condition rather than on the screen.
//
// One test needs it, and the reason is worth writing down: a frame drawn
// between tea.Exec handing the terminal back and the renderer's next flush does
// not reliably reach the output. The model is right — the message is on the
// final frame every time — but the intermediate byte stream is not something to
// synchronise on, so the editor's test waits for the fake to have been called
// and reads the frame the program ended on.
func pressUntil(
	t *testing.T, in ui.Input, width, height int, keys []string, until func() bool,
) string {
	t.Helper()

	return script{width: width, height: height, keys: keys, settled: until}.run(t, in)
}

// script is one run of the interface: wait, type, wait, quit.
type script struct {
	width, height int
	before        string
	keys          []string
	after         string
	settled       func() bool
}

// run drives one bubbletea program over a pair of buffers, which is what `isu
// ui` itself does with a pipe on either side of it.
//
// teatest is used where M6-S1 asks for it — the golden frames and the
// assertion that `q` ends the program — and not here. tea.Exec, which is how
// `n` hands the terminal to an editor, does not come back reliably under
// teatest's harness: measured at about one run in two, the program released the
// terminal and never repainted, and waiting for the first frame before typing
// made it every run. The same program over an ordinary pair of buffers did not
// fail in two hundred, and internal/cli asserts the editor end to end through
// the real command as well.
func (s script) run(t *testing.T, in ui.Input) string {
	t.Helper()

	input, output := &buffer{}, &buffer{}
	p := tea.NewProgram(ui.New(in), tea.WithInput(input), tea.WithOutput(output))

	type ended struct {
		model tea.Model
		err   error
	}

	finished := make(chan ended, 1)

	go func() {
		last, err := p.Run()
		finished <- ended{model: last, err: err}
	}()

	p.Send(tea.WindowSizeMsg{Width: s.width, Height: s.height})

	// Nothing is typed at a program that has not drawn yet: the header is on
	// every frame, so waiting for it is waiting for the event loop to be
	// running and the renderer to have started.
	await(t, output, "isu  ")
	await(t, output, s.before)

	for _, name := range s.keys {
		p.Send(key(name))
	}

	await(t, output, s.after)
	settle(t, s.settled)

	p.Send(key("q"))

	select {
	case done := <-finished:
		require.NoError(t, done.err)

		final, ok := done.model.(ui.Model)
		require.True(t, ok)

		return final.View()
	case <-time.After(5 * time.Second):
		p.Kill()
		require.Fail(t, "the interface did not quit", "last frame:\n%s", output.String())

		return ""
	}
}

// await holds until the screen says something, and returns at once when there
// is nothing to wait for.
func await(t *testing.T, out *buffer, until string) {
	t.Helper()

	if until == "" {
		return
	}

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if strings.Contains(out.String(), until) {
			return
		}

		time.Sleep(2 * time.Millisecond)
	}

	require.Failf(t, "the interface never said it", "waiting for %q in:\n%s",
		until, out.String())
}

// settle holds until a condition about the fake is true, and returns at once
// when there is nothing to wait for.
func settle(t *testing.T, until func() bool) {
	t.Helper()

	if until == nil {
		return
	}

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if until() {
			// One more turn of the event loop, so that whatever the action
			// asked for next has been applied before the frame is read.
			time.Sleep(20 * time.Millisecond)

			return
		}

		time.Sleep(2 * time.Millisecond)
	}

	require.Fail(t, "the interface never got there")
}

// buffer is a terminal's worth of bytes, written by the program's renderer and
// read by the test. Both happen on their own goroutines, so it is locked.
type buffer struct {
	mu      sync.Mutex
	written bytes.Buffer
}

func (b *buffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.written.Read(p)
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.written.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.written.String()
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

// M6-S4 asks for a golden frame per type, because what a type requires
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
	in.Actions = &wired{held: ui.Folder{
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
	in.Actions = &wired{folderErr: errors.New("permission denied")}

	require.Contains(t, detailPane(drive(t, in, 110, 30, "permission denied")),
		"permission denied")
}

// M6-S4: "an issue with twenty attachments scrolls".
func TestAnIssueWithTwentyAttachmentsScrolls(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, 20)
	for i := range 20 {
		names = append(names, fmt.Sprintf("capture-%02d.har", i))
	}

	in := oneIssue(item("ISU-manybit", "Twenty things beside it"))
	in.Actions = &wired{held: ui.Folder{Attachments: names}}

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
