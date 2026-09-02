package cli

import (
	"context"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/ui"
)

func (a *app) uiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "the whole board, on one screen",
		Long: `ui opens the terminal interface: the board on the left, everything about
what the cursor is on down the right, and the same derived statuses every other
command prints.

It reads the repository once, at startup, exactly as ` + "`isu board`" + ` does. Actions
taken from it — claiming, filing, checking a branch out — run the same code the
commands run and re-read the repository afterwards, so what is on the screen is
what the refs say and not a cache of what they said.

--json prints what the interface would open on, which is ` + "`isu board`" + `'s payload:
an interface is not a thing an agent can read, and refusing to answer would
leave one guessing.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.ui(cmd)
		},
	}
}

func (a *app) ui(cmd *cobra.Command) error {
	ctx := cmd.Context()

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	v, err := s.view(ctx)
	if err != nil {
		return err
	}

	if a.asJSON {
		return emit(a.env.Stdout, BoardPayload{
			Trunk:     v.trunkName,
			Refs:      v.refNames(),
			Freshness: v.freshness,
			Groups:    v.groups(),
		})
	}

	return a.runProgram(ctx, ui.New(a.uiInput(s, v)))
}

// uiInput is the whole repository, handed over.
//
// The groups are the ones `isu board` renders, from the same function, which is
// what PLAN.md M6-S2 means by the grouping matching exactly: not two orderings
// that agree, one ordering used twice.
func (a *app) uiInput(s *session, v *view) ui.Input {
	loaded := v.itemGroups()
	groups := make([]ui.Group, 0, len(loaded))

	for _, group := range loaded {
		groups = append(groups, ui.Group{Status: group.status, Items: group.items})
	}

	return ui.Input{
		Data:     uiData(s, v, groups),
		Actions:  &actions{app: a, session: s},
		Renderer: a.rendererFor(a.env.Stdout),
	}
}

// uiData is one derivation of the repository, as the interface reads it. A
// reload replaces this and nothing else — see ui.Data.
func uiData(s *session, v *view, groups []ui.Group) ui.Data {
	return ui.Data{
		Trunk:     v.trunkName,
		Board:     v.board,
		Groups:    groups,
		Ready:     readyItems(v),
		Freshness: freshnessLine(v.freshness, s.cfg.FetchWarnAfter()),
		Now:       v.now,
	}
}

// rendererFor decides whether the interface may use colour, and it is the same
// decision themeFor makes for every other command: lipgloss reads the profile
// of what it is writing to, so a renderer over a terminal has colour in it and
// one over a pipe does not.
func (a *app) rendererFor(w io.Writer) *lipgloss.Renderer {
	if a.themeFor(w).color {
		return lipgloss.NewRenderer(w)
	}

	return lipgloss.NewRenderer(io.Discard)
}

// runProgram is the event loop, and the one place in this package that owns a
// terminal.
//
// The alt screen is what makes `q` leave the shell as it found it: the
// interface draws on a second screen buffer and the terminal restores the first
// on the way out, so the scrollback somebody was reading before they typed `isu
// ui` is still there afterwards.
func (a *app) runProgram(ctx context.Context, m tea.Model) error {
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(a.env.stdin()),
		tea.WithOutput(a.env.Stdout),
		tea.WithAltScreen(),
	)

	_, err := p.Run()

	return err
}

// stdin is where the interface reads keys from. Nil means the process's own,
// for the same reason every other stream in Env works that way.
func (e Env) stdin() io.Reader {
	if e.Stdin == nil {
		return os.Stdin
	}

	return e.Stdin
}
