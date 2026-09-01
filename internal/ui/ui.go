// Package ui is isu's terminal interface: one bubbletea program over a board
// somebody else derived.
//
// Nothing here loads anything. The whole repository arrives as an Input — the
// groups `isu board` renders, the derived board behind them, and the moment
// they were derived at — and every frame is a pure function of that plus which
// keys have been pressed since. Anything the interface cannot answer from what
// it was handed goes back out through Actions, which internal/cli implements
// with the same code paths its commands run.
//
// That layering is the point rather than a tidiness: PLAN.md M6-S5 asks for a
// test that fails if this package calls git, and a package that cannot load
// anything cannot call git by accident. It is the same rule internal/model and
// internal/check are held to, for the same reason — a derivation that could run
// a git process would run one per issue the first time somebody was in a hurry,
// and a renderer that could run one would run it once per frame.
package ui

import (
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dgorshkov/isu/internal/model"
)

// Input is everything the interface reads.
type Input struct {
	// Trunk is what to call the ref the board was derived from.
	Trunk string
	// Board is the derivation the groups were taken from. It is what an id in
	// `parent` or `blocked_by` is resolved against, and nothing else — a pane
	// that resolved ids itself would be a pane that loaded a repository.
	Board *model.Board
	// Groups are the issues by derived status, in the precedence order of
	// PLAN.md's table — the same grouping `isu board` renders, built by the
	// same function, so that two screens showing one repository cannot
	// disagree about it.
	Groups []Group
	// Ready is the queue `r` shows: what `isu ready` would print.
	Ready []*model.Item
	// Freshness is the sentence about how old the remote refs are, already
	// worded by the caller — it is the one line on this screen that is about
	// the clone rather than about the work.
	Freshness string
	// Now is what ages are measured against.
	Now time.Time
	// Renderer decides whether this screen may use colour. Nil means it may
	// not, which is what a test wants and what a pipe deserves.
	Renderer *lipgloss.Renderer
}

// Group is one status's issues, in the order `isu board` renders them.
type Group struct {
	Status model.Status
	Items  []*model.Item
}

// Model is the whole interface: what it was handed, and where the user is in
// it.
type Model struct {
	in     Input
	styles styles

	width  int
	height int

	// rows is the list as it is drawn: headings, issues, and the indentation
	// that puts a child under its epic. It is rebuilt whenever something
	// changes what the list holds, and never mid-frame.
	rows []row
	// cursor is an index into rows, and always lands on an issue when there is
	// one to land on.
	cursor int
	// top is the first row drawn, which is how the list scrolls.
	top int

	// ready says the list is showing the ready queue rather than the board.
	ready bool

	// filter is the needle, lowercased, and filtering says its line is open.
	// A needle survives its line closing: what somebody has narrowed to is the
	// list they wanted.
	filter    string
	filtering bool
	// collapsed holds the epics that are folded, by id.
	collapsed map[string]bool
	// search is every issue's filterable text, lowercased once — see
	// searchable.
	search map[string]string
	// sticky is the issue somebody last chose, which is not always the issue
	// the cursor is on: a filter can hide it. It is what the cursor goes back
	// to when the filter that hid it goes, and it is only ever set by a
	// deliberate move.
	sticky string
}

// The size a terminal is assumed to be until it says otherwise. PLAN.md asks
// for 80×24 and that is the smallest terminal anybody still ships, so it is
// also the honest guess for the one frame drawn before the first resize.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// New builds the interface over one derivation of a repository.
func New(in Input) Model {
	m := Model{
		in:        in,
		styles:    newStyles(in.Renderer),
		width:     defaultWidth,
		height:    defaultHeight,
		collapsed: map[string]bool{},
		search:    index(in.Groups, in.Ready),
	}

	m.rebuild()

	return m
}

// Init is what bubbletea runs first. There is nothing to start: everything this
// screen draws was handed to it.
func (m Model) Init() tea.Cmd { return nil }

// Update is the whole key map and the resize.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()

		return m, nil
	case tea.KeyMsg:
		return m.key(msg)
	}

	return m, nil
}

// key is one keypress.
//
// The filter line comes first and takes every printable key, so the whole
// command map below is unreachable while it is open. That is the point of it:
// see typeInto.
func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	name := msg.String()

	if m.filtering {
		if next, handled := m.typeInto(name, msg.Runes); handled {
			return next, nil
		}
	}

	switch name {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "/":
		m.filtering = true

		return m, nil
	case "esc":
		m.filter = ""
		m.rebuild()

		return m, nil
	case "r":
		m.ready = !m.ready
		m.rebuild()

		return m, nil
	case "up", "k":
		return m.move(-1), nil
	case "down", "j":
		return m.move(1), nil
	case "pgup":
		return m.move(-m.bodyHeight()), nil
	case "pgdown":
		return m.move(m.bodyHeight()), nil
	case "home":
		return m.jump(false), nil
	case "end":
		return m.jump(true), nil
	case "left", "h":
		return m.fold(), nil
	case "right", "l":
		return m.unfold(), nil
	}

	return m, nil
}

// View is one frame, exactly as tall as the terminal and never wider.
//
// Every line is trimmed on the right, which is not cosmetic: a pane padded to
// its width leaves a run of spaces that is invisible on a terminal and very
// loud in a golden file, and this package's golden files are its review.
func (m Model) View() string {
	frame := make([]string, 0, m.height)
	frame = append(frame, m.header()...)
	frame = append(frame, m.body()...)
	frame = append(frame, m.footer()...)

	return strings.Join(fit(frame, m.width, m.height), "\n")
}

// body is the two panes side by side: the list on the left, and everything
// about what the cursor is on down the right.
func (m Model) body() []string {
	height := m.bodyHeight()
	left := m.list(m.listWidth(), height)
	right := m.detail(m.detailWidth(), height)

	out := make([]string, 0, height)

	for i := range height {
		out = append(out, column(at(left, i), m.listWidth())+gutter+at(right, i))
	}

	return out
}

// at is one line of a pane, or nothing where the pane has run out.
func at(pane []string, i int) string {
	if i < len(pane) {
		return pane[i]
	}

	return ""
}

// date is how a frame writes a day. An issue is not filed at a moment: it is
// filed on a day, which is what its age is computed from.
func date(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.Format(time.DateOnly)
}

// fit makes a frame exactly the size of the terminal it is drawn for. A line
// too many scrolls the header off the top on every redraw; a line too few
// leaves the previous frame's footer sitting under this one.
func fit(frame []string, width, height int) []string {
	out := make([]string, 0, height)

	for _, line := range frame {
		out = append(out, strings.TrimRight(truncate(line, width), " "))
	}

	for len(out) < height {
		out = append(out, "")
	}

	return out[:height]
}

// truncate cuts a line to a width, in characters rather than bytes, and says so
// with an ellipsis when it had to.
func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}

	if width <= 1 {
		return string(runes[:max(width, 0)])
	}

	return string(runes[:width-1]) + "…"
}

// atLeast is the floor every derived width and height gets. A terminal narrow
// enough to make one of them negative is a terminal this interface cannot serve
// well, but it must not be one it crashes on.
func atLeast(n, floor int) int {
	if n < floor {
		return floor
	}

	return n
}

// discard is the writer a colourless renderer is built over. lipgloss decides a
// profile from what it is writing to, and what this is writing to is not a
// terminal, so the profile is the one with no colour in it — which is exactly
// the decision internal/cli's theme makes for a pipe, made the same way.
var discard io.Writer = io.Discard
