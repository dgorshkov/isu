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
// That layering is the point rather than a tidiness: M6-S5 asks for a
// test that fails if this package calls git, and a package that cannot load
// anything cannot call git by accident. It is the same rule internal/model and
// internal/check are held to, for the same reason — a derivation that could run
// a git process would run one per issue the first time somebody was in a hurry,
// and a renderer that could run one would run it once per frame.
package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/dgorshkov/isu/internal/model"
)

// Input is everything the interface reads: one derivation of a repository, the
// things it cannot do itself, and whether it may use colour.
type Input struct {
	Data
	// Actions are the things the interface cannot do itself. A nil Actions is a
	// read-only interface: everything derived is still on the screen, and what
	// has to be loaded or written is not.
	Actions Actions
	// Renderer decides whether this screen may use colour. Nil means it may
	// not, which is what a test wants and what a pipe deserves.
	Renderer *lipgloss.Renderer
}

// Data is the derivation, and the only part of an Input a reload replaces.
//
// It is separate from the wiring because a reload is a re-read of a repository
// and not a rebuild of the interface: the streams, the palette and the actions
// are the same ones, and the filter, the fold and the cursor are meant to
// survive somebody pressing `c`.
type Data struct {
	// Trunk is what to call the ref the board was derived from.
	Trunk string
	// Board is the derivation the groups were taken from. It is what an id in
	// `parent` or `blocked_by` is resolved against, and nothing else — a pane
	// that resolved ids itself would be a pane that loaded a repository.
	Board *model.Board
	// Groups are the issues by derived status, in the precedence order of
	// the plan's table — the same grouping `isu board` renders, built by the
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
	// Actions are the things the interface cannot do itself. A nil Actions is a
	// read-only interface: everything derived is still on the screen, and what
	// has to be loaded or written is not.
	Actions Actions
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

	// focus says which pane has the keys. `enter` gives them to the detail so
	// that it can be scrolled; `esc` gives them back.
	focus focus
	// detailTop is the first line of the detail pane that is drawn.
	detailTop int
	// folders holds what has arrived from Actions.Folder, and asked holds what
	// has been sent for — one is not the other, because a request that failed
	// must not be sent again on every frame that follows it.
	folders map[string]held
	asked   map[string]bool
	// md renders the body. It is rebuilt on a resize, because the wrap width is
	// the pane's, and it is nil on a pane too narrow to wrap into.
	md *glamour.TermRenderer

	// said is what the last action reported, which is the footer's middle line.
	// An action that said nothing is one nobody can tell happened.
	said string
	// working says an action is in flight, so that the footer does not read as
	// though nothing was pressed.
	working bool
	// quitting is `q` pressed while an action was in flight. Leaving in the
	// middle of a push would abandon it half way through the one operation in
	// this product that has to be atomic, so the key is remembered and acted on
	// when the action reports.
	quitting bool
}

// focus is which pane the keys go to.
type focus uint8

const (
	onList focus = iota
	onDetail
)

// held is one answer from Actions.Folder, including the answer that it could
// not be read — never failing silently means keeping the failure, not dropping
// it.
type held struct {
	folder Folder
	err    error
}

// The size a terminal is assumed to be until it says otherwise. The plan asks
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
		folders:   map[string]held{},
		asked:     map[string]bool{},
	}

	m.md = newMarkdown(m.detailWidth(), in.Renderer != nil)
	m.rebuild()

	return m
}

// Init asks for what lives beside the issue the cursor opens on. Everything
// else this screen draws was handed to it.
func (m Model) Init() tea.Cmd { return m.fetch(m.selectedID()) }

// Update is the whole key map and the resize.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.md = newMarkdown(m.detailWidth(), m.in.Renderer != nil)
		m.scroll()

		return m, nil
	case folderMsg:
		m.folders[msg.id] = held{folder: msg.folder, err: msg.err}

		return m, nil
	case doneMsg:
		return m.done(msg)
	case reloadMsg:
		return m.reloaded(msg)
	case tea.KeyMsg:
		next, cmd := m.keys(msg)

		return next, cmd
	}

	return m, nil
}

// keys applies one key message, which is not always one keypress.
//
// A burst of printable characters arrives as a single message carrying several
// runes: that is how a terminal delivers a paste, and how it delivers somebody
// typing faster than the read loop drains. Each rune is one keypress here. A
// `j` that did nothing because it arrived in the same read as the `c` after it
// would make the interface drop keys under exactly the condition somebody is
// going fast, and a pasted needle would be one key the filter had never heard
// of.
func (m Model) keys(msg tea.KeyMsg) (Model, tea.Cmd) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) <= 1 {
		return m.key(msg)
	}

	var cmds []tea.Cmd

	for _, r := range msg.Runes {
		next, cmd := m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})

		m = next

		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// key is one keypress.
//
// The filter line comes first and takes every printable key, so the whole
// command map below is unreachable while it is open. That is the point of it:
// see typeInto.
func (m Model) key(msg tea.KeyMsg) (Model, tea.Cmd) {
	name := msg.String()

	if m.filtering {
		if next, handled := m.typeInto(name, msg.Runes); handled {
			return next, nil
		}
	}

	// `q` leaves, from wherever it is pressed and whatever has the keys. A key
	// that quits from one pane and does something else from another is the one
	// thing a person has to keep in their head, and this map is meant to be
	// forgettable.
	if name == "q" || name == "ctrl+c" {
		if m.working {
			m.quitting = true

			return m, nil
		}

		return m, tea.Quit
	}

	if m.focus == onDetail {
		return m.scrollDetail(name)
	}

	switch name {
	case "enter":
		m.focus = onDetail

		return m, nil
	case "c":
		return m.act(claiming)
	case "g":
		return m.act(going)
	case "n":
		return m.file()
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
		return m.moved(-1)
	case "down", "j":
		return m.moved(1)
	case "pgup":
		return m.moved(-m.bodyHeight())
	case "pgdown":
		return m.moved(m.bodyHeight())
	case "home":
		return m.moved(-len(m.rows))
	case "end":
		return m.moved(len(m.rows))
	case "left", "h":
		return m.fold(), nil
	case "right", "l":
		return m.unfold(), nil
	}

	return m, nil
}

// moved steps the cursor and asks for whatever now sits beside it.
func (m Model) moved(delta int) (Model, tea.Cmd) {
	next := m.move(delta)

	return next, next.fetch(next.selectedID())
}

// scrollDetail is the key map while the detail pane has the keys.
//
// It is a short map on purpose: what somebody wants from a pane they have just
// opened is to move up and down it and then to get out, and every key that is
// not one of those three is better spent on the list.
func (m Model) scrollDetail(name string) (Model, tea.Cmd) {
	switch name {
	case "esc", "enter":
		m.focus = onList
		m.detailTop = 0
	case "up", "k":
		m.detailTop = max(m.detailTop-1, 0)
	case "down", "j":
		m.detailTop = min(m.detailTop+1, m.detailFloor())
	case "pgup":
		m.detailTop = max(m.detailTop-m.bodyHeight(), 0)
	case "pgdown":
		m.detailTop = min(m.detailTop+m.bodyHeight(), m.detailFloor())
	case "home":
		m.detailTop = 0
	case "end":
		m.detailTop = m.detailFloor()
	}

	return m, nil
}

// detailFloor is as far as the pane can be scrolled: the point where its last
// line is on the last row. Scrolling past the end of a document is how a reader
// loses the document.
func (m Model) detailFloor() int {
	return max(len(m.pane(m.detailWidth()))-m.bodyHeight(), 0)
}

// View is one frame, exactly as tall as the terminal and never wider.
//
// Every line is trimmed on the right, which is not cosmetic: a pane padded to
// its width leaves a run of spaces that is invisible on a terminal and very
// loud in a golden file, and this package's golden files are its review.
func (m Model) View() string {
	frame := make([]string, 0, m.height)
	frame = append(frame, m.header()...)
	frame = append(frame, m.panes()...)
	frame = append(frame, m.footer()...)

	return strings.Join(fit(frame, m.width, m.height), "\n")
}

// panes is the two of them side by side: the list on the left, and everything
// about what the cursor is on down the right.
func (m Model) panes() []string {
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
	out := make([]string, height)

	for i := range min(len(frame), height) {
		out[i] = strings.TrimRight(truncate(frame[i], width), " ")
	}

	return out
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
