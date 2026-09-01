package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/dgorshkov/isu/internal/model"
)

// Colour is a decision about the terminal rather than about the data, and it is
// made the same way internal/cli's theme makes it: lipgloss reads the profile of
// whatever it is writing to, so a renderer built over a pipe has no colour in it
// and one built over a terminal has as much as the terminal admits to. A nil
// renderer here is the colourless one, which is what every golden frame in this
// package is rendered through.
//
// The palette is four colours and no more. A status list where every row is a
// different colour is a list where the colour means nothing, so the two that
// carry a decision — somebody is on it, nobody can start it — are the two that
// are coloured, and the rest are the terminal's own foreground.
type styles struct {
	title    lipgloss.Style
	dim      lipgloss.Style
	heading  lipgloss.Style
	selected lipgloss.Style
	status   map[model.Status]lipgloss.Style
	plain    lipgloss.Style
}

// The ANSI colours the palette is drawn from. They are the sixteen-colour
// numbers rather than hex, so that they follow whatever the terminal's own
// theme says those colours are.
const (
	colorGreen  = lipgloss.Color("2")
	colorYellow = lipgloss.Color("3")
	colorBlue   = lipgloss.Color("4")
	colorGrey   = lipgloss.Color("8")
)

func newStyles(r *lipgloss.Renderer) styles {
	if r == nil {
		r = lipgloss.NewRenderer(discard)
	}

	plain := r.NewStyle()

	s := styles{
		title:    plain.Bold(true),
		dim:      plain.Foreground(colorGrey),
		heading:  plain.Bold(true),
		selected: plain.Reverse(true),
		plain:    plain,
	}

	s.status = map[model.Status]lipgloss.Style{
		model.StatusDone:           plain.Foreground(colorGreen),
		model.StatusDropped:        plain.Foreground(colorGrey),
		model.StatusAwaitingTriage: plain.Foreground(colorYellow),
		model.StatusInProgress:     plain.Foreground(colorBlue),
		model.StatusReopened:       plain.Foreground(colorYellow),
		model.StatusOpen:           plain,
	}

	return s
}

// forStatus is how a row is coloured. A status the table does not name is drawn
// plain rather than not drawn: the board has to render the row either way.
func (s styles) forStatus(status model.Status) lipgloss.Style {
	if style, ok := s.status[status]; ok {
		return style
	}

	return s.plain
}
