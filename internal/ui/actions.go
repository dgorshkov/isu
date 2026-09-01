package ui

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dgorshkov/isu/internal/model"
)

// The three things this interface does to a repository, and one re-read.
//
// Every one of them is internal/cli's implementation of the command with the
// same name, called through Actions. That is PLAN.md M6-S5's requirement and it
// is also the only design this package could have: it holds a derivation and no
// way to make another one, so a `c` that did not go back out through here would
// have nothing to write with.

// Streams are the three the editor is handed. The interface releases the
// terminal for as long as it runs — an editor that cannot read the keyboard is
// an editor nobody can type into.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// what is which action ran, so that one message type can carry all of them.
type what string

const (
	claiming what = "claim"
	going    what = "branch"
	filing   what = "new"
)

// doneMsg is an action having finished, either way.
type doneMsg struct {
	what what
	said string
	err  error
}

// reloadMsg is the repository, read again after something changed it.
type reloadMsg struct {
	data Data
	err  error
}

// act runs the one action that takes an id, and there are two of them.
func (m Model) act(which what) (tea.Model, tea.Cmd) {
	item := m.selected()
	if stopped, why := m.cannot(item); why != "" {
		stopped.said = why

		return stopped, nil
	}

	id := item.ID
	actions := m.in.Actions

	run := actions.Claim
	if which == going {
		run = actions.Goto
	}

	m.working = true
	m.said = "…"

	return m, func() tea.Msg {
		said, err := run(id)

		return doneMsg{what: which, said: said, err: err}
	}
}

// file opens the editor on a new issue.
//
// tea.Exec is what hands the terminal over and takes it back: the interface
// leaves the alt screen, the editor draws on the terminal somebody is actually
// looking at, and the frame comes back when it exits.
func (m Model) file() (tea.Model, tea.Cmd) {
	if m.in.Actions == nil {
		m.said = readOnly

		return m, nil
	}

	editor := &suspended{run: m.in.Actions.New}

	m.working = true
	m.said = "…"

	return m, tea.Exec(editor, func(err error) tea.Msg {
		return doneMsg{what: filing, said: editor.said, err: err}
	})
}

// cannot is why an action will not run, and is empty when it will.
func (m Model) cannot(item *model.Item) (Model, string) {
	switch {
	case m.in.Actions == nil:
		return m, readOnly
	case item == nil:
		return m, "nothing selected: there is no issue here to act on"
	default:
		return m, ""
	}
}

// readOnly is what an interface with nothing wired to it says. Saying it is the
// difference between a tool that is read-only and one that is broken.
const readOnly = "read-only: this interface was opened without the commands behind it"

// done is an action reporting, and it always reports. "Never fails silently" is
// M6-S5's rule and this is the whole of it: one message line, written by
// whichever half of the action finished.
func (m Model) done(msg doneMsg) (tea.Model, tea.Cmd) {
	m.working = false

	if msg.err != nil {
		m.said = string(msg.what) + ": " + msg.err.Error()

		return m, m.leaveIfAsked()
	}

	m.said = msg.said
	if m.said == "" {
		m.said = string(msg.what) + ": nothing to do"
	}

	if m.quitting {
		return m, tea.Quit
	}

	return m, m.reload()
}

// leaveIfAsked quits if `q` was pressed while the action was still running.
func (m Model) leaveIfAsked() tea.Cmd {
	if m.quitting {
		return tea.Quit
	}

	return nil
}

// reload re-reads the repository, because every one of these actions changes
// what it says.
func (m Model) reload() tea.Cmd {
	actions := m.in.Actions

	return func() tea.Msg {
		data, err := actions.Reload()

		return reloadMsg{data: data, err: err}
	}
}

// reloaded swaps the derivation and keeps everything about where somebody was.
//
// The filter, the fold and the cursor all survive, which is why Data is
// separate from Input: a reload is a re-read of a repository and not a rebuild
// of the interface.
func (m Model) reloaded(msg reloadMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.said = "re-reading the repository: " + msg.err.Error()

		return m, nil
	}

	m.in.Data = msg.data
	m.search = index(msg.data.Groups, msg.data.Ready)
	m.folders = map[string]held{}
	m.asked = map[string]bool{}
	m.rebuild()

	return m, m.fetch(m.selectedID())
}

// suspended is something that needs the terminal, wrapped so that bubbletea can
// hand it over. It is tea.ExecCommand, and it is how this package runs an
// editor without ever building a process.
type suspended struct {
	run     func(Streams) (string, error)
	streams Streams
	said    string
}

func (s *suspended) Run() error {
	said, err := s.run(s.streams)
	s.said = said

	return err
}

func (s *suspended) SetStdin(r io.Reader)  { s.streams.In = r }
func (s *suspended) SetStdout(w io.Writer) { s.streams.Out = w }
func (s *suspended) SetStderr(w io.Writer) { s.streams.Err = w }
