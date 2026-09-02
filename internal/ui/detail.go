package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/dgorshkov/isu/internal/model"
)

// Actions are the things the interface cannot do itself.
//
// Everything on this interface is implemented by internal/cli with the code its
// commands run, which is PLAN.md M6-S5's whole requirement: the interface
// shares command implementations with the CLI rather than reimplementing them,
// and it is a package that cannot load anything, so it could not reimplement
// them if it wanted to.
type Actions interface {
	// Folder is what lives beside an issue: the attachments, and the comments.
	// It is `isu show`'s own loader.
	Folder(id string) (Folder, error)
	// Claim says somebody is working on an issue, and is `isu claim`.
	Claim(id string) (string, error)
	// Goto checks out the branch claiming an issue.
	Goto(id string) (string, error)
	// New files an issue through the user's editor, which is why it is handed
	// the terminal.
	New(streams Streams) (string, error)
	// Reload re-derives the repository, after one of the three above changed
	// what it says.
	Reload() (Data, error)
}

// Folder is what lives beside an issue's README.
type Folder struct {
	// Attachments are the files beside it, by name. Their contents are
	// arbitrary bytes and are not read.
	Attachments []string
	Comments    []Comment
}

// Comment is one comment file: what it is called, and what it says.
type Comment struct {
	Name string
	Body string
}

// folderMsg is a folder arriving, which happens after the frame that asked for
// it. The id comes back with it because the cursor may have moved on.
type folderMsg struct {
	id     string
	folder Folder
	err    error
}

// fetch asks for what lives beside an issue, once.
//
// Once, because the answer costs a git process and the cursor walks over the
// same issue every time somebody scrolls past it. An interface with no actions
// wired to it asks for nothing and draws the rest.
func (m *Model) fetch(id string) tea.Cmd {
	if m.in.Actions == nil || id == "" || m.asked[id] {
		return nil
	}

	m.asked[id] = true
	actions := m.in.Actions

	return func() tea.Msg {
		held, err := actions.Folder(id)

		return folderMsg{id: id, folder: held, err: err}
	}
}

// The detail pane answers "can I start this?" without leaving the interface,
// which is PLAN.md M6-S4's whole sentence for it. Everything it needs about
// the issue itself is on the item; everything it needs about another issue —
// what a blocker is called and whether it has finished — comes from the board
// the caller derived, because a pane that resolved ids itself would be a pane
// that loaded a repository.
func (m Model) detail(width, height int) []string {
	pane := m.pane(width)

	top := min(m.detailTop, max(len(pane)-height, 0))

	return pane[top:min(top+height, len(pane))]
}

// pane is the whole of the detail, before it is cut to what fits.
func (m Model) pane(width int) []string {
	item := m.selected()
	if item == nil {
		return []string{m.styles.dim.Render("nothing selected")}
	}

	out := []string{m.styles.title.Render(item.ID), titleOf(item), ""}

	out = append(out, fields(m.fieldRows(item), width)...)

	for _, claim := range item.Claims {
		out = append(out, "", m.styles.dim.Render(claimLine(claim)))
	}

	out = append(out, m.children(item, width)...)
	out = append(out, m.body(item)...)
	out = append(out, m.beside(item, width)...)

	return wrapAll(out, width)
}

// children is an epic's own fold, listed. An epic is the one issue whose answer
// to "can I start this?" is entirely about other issues.
func (m Model) children(item *model.Item, width int) []string {
	if item.Epic == nil {
		return nil
	}

	out := []string{"", m.styles.title.Render("children")}

	rows := make([][2]string, 0, len(item.Epic.Children))

	for _, id := range item.Epic.Children {
		child, ok := m.lookup(id)
		if !ok {
			rows = append(rows, [2]string{id, "(no issue with this id)"})

			continue
		}

		rows = append(rows, [2]string{id, string(child.Status) + "  " + titleOf(child)})
	}

	if len(rows) == 0 {
		return append(out, "  "+m.styles.dim.Render("none: this epic folds over nothing"))
	}

	for _, line := range fields(rows, atLeast(width-2, 1)) {
		out = append(out, "  "+line)
	}

	return out
}

// body is the markdown below the frontmatter, rendered for a terminal.
//
// `isu show` prints it verbatim and says so; this is the pane that does not
// have to, and glamour is in PLAN.md's allowlist for exactly this.
func (m Model) body(item *model.Item) []string {
	if item.Issue == nil || strings.TrimSpace(item.Issue.Body) == "" {
		return nil
	}

	return append([]string{""}, m.markdown(item.Issue.Body)...)
}

// beside is what lives in the issue's folder, once it has arrived.
func (m Model) beside(item *model.Item, width int) []string {
	beside, arrived := m.folders[item.ID]
	if !arrived {
		return nil
	}

	if beside.err != nil {
		return []string{
			"",
			m.styles.title.Render("beside it"),
			"could not be read: " + beside.err.Error(),
		}
	}

	var out []string

	if len(beside.folder.Attachments) > 0 {
		out = append(out, "", m.styles.title.Render("attachments"))
		for _, name := range beside.folder.Attachments {
			out = append(out, "  "+name)
		}
	}

	for _, comment := range beside.folder.Comments {
		out = append(out, "", m.styles.title.Render("comment "+comment.Name))
		out = append(out, wrapAll(lines(strings.TrimRight(comment.Body, "\n")), width)...)
	}

	return out
}

// markdown renders a body, and prints it as it was written when it cannot.
//
// A terminal too narrow to wrap into is the reachable half of that: glamour
// wraps to a width, and below markdownFloor the width is not one anything can
// be wrapped to. The other half is a renderer that will not build or will not
// render, which is a style name this package chose and a width it computed —
// so it cannot fail on anything a user did, and the body is printed rather than
// lost, which is what `isu show` does anyway.
func (m Model) markdown(text string) []string {
	if m.md == nil {
		return lines(strings.TrimRight(text, "\n"))
	}

	out, err := m.md.Render(text)
	if err != nil {
		return lines(strings.TrimRight(text, "\n"))
	}

	trimmed := make([]string, 0, 8)
	for _, line := range lines(strings.Trim(out, "\n")) {
		trimmed = append(trimmed, strings.TrimRight(line, " "))
	}

	return trimmed
}

// markdownFloor is the narrowest pane glamour is asked to wrap into.
const markdownFloor = 40

// newMarkdown builds the renderer for a pane of a given width.
//
// The style is the plain one unless the screen has colour in it, for the same
// reason every other style here is: what this is written to decides, and a
// frame written into a pipe has no colour in it.
func newMarkdown(width int, color bool) *glamour.TermRenderer {
	if width < markdownFloor {
		return nil
	}

	style := "ascii"
	if color {
		style = "auto"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style), glamour.WithWordWrap(width))
	if err != nil {
		return nil
	}

	return r
}

// lines splits rendered markdown the way a pane reads it.
func lines(s string) []string { return strings.Split(s, "\n") }

// fieldRows is the block of key/value lines under the title. A field the file
// does not carry is not printed: an issue is not a form, and an empty row for
// every optional key would bury the three that are filled in.
func (m Model) fieldRows(item *model.Item) [][2]string {
	rows := [][2]string{{"status", statusOf(item)}}

	rows = appendRow(rows, "type", typeOf(item))
	rows = appendRow(rows, "priority", priorityOf(item))

	if item.Issue != nil {
		i := item.Issue

		rows = appendRow(rows, "state", string(i.State))
		rows = appendRow(rows, "owner", i.Owner)
		rows = appendRow(rows, "created", date(i.Created))
		rows = appendRow(rows, "repro", i.Repro)
		rows = appendRow(rows, "acceptance", i.Acceptance)
		rows = appendRow(rows, "question", i.Question)
		rows = appendRow(rows, "reason", i.Reason)
		rows = appendRow(rows, "resolution", string(i.Resolution))

		if i.Parent != "" {
			rows = append(rows, [2]string{"parent", m.link(i.Parent)})
		}

		for n, blocker := range i.BlockedBy {
			rows = append(rows, [2]string{key("blocked_by", n), m.link(blocker)})
		}
	}

	if position := m.position(item); position != "" {
		rows = append(rows, [2]string{"position", position})
	}

	for n, ref := range item.Elsewhere {
		rows = append(rows, [2]string{key("branches", n), ref})
	}

	if item.Broken != nil {
		rows = append(rows, [2]string{"broken", item.Broken.Err.Error()})
	}

	return rows
}

// position is where an issue sits among its epic's children, which is the
// second half of "which epic is this in" — an issue that is one of forty is a
// different proposition from one that is the last of three.
func (m Model) position(item *model.Item) string {
	epic, ok := m.lookup(parentOf(item))
	if !ok || epic.Epic == nil {
		return ""
	}

	for i, id := range epic.Epic.Children {
		if id == item.ID {
			return strconv.Itoa(i+1) + " of " + strconv.Itoa(len(epic.Epic.Children))
		}
	}

	return ""
}

// key names a repeated field once. A column of `blocked_by` down the side of a
// pane says the same word four times and the ids once.
func key(name string, n int) string {
	if n == 0 {
		return name
	}

	return ""
}

func appendRow(rows [][2]string, name, value string) [][2]string {
	if value == "" {
		return rows
	}

	return append(rows, [2]string{name, value})
}

// link resolves an id another issue named, so that a blocker reads as work
// rather than as a token. An id the board does not hold is M5-S2's to report
// and is shown as what it is rather than dropped.
func (m Model) link(id string) string {
	item, ok := m.lookup(id)
	if !ok {
		return id + "  (no issue with this id)"
	}

	return id + "  " + titleOf(item) + " (" + string(item.Status) + ")"
}

// lookup is the board, where there is one. An interface handed no board can
// still draw every issue it was given; what it cannot do is say what an id
// points at.
func (m Model) lookup(id string) (*model.Item, bool) {
	if m.in.Board == nil {
		return nil, false
	}

	return m.in.Board.Get(id)
}

// fields lays the key/value block out so the values line up, and keeps them
// lined up when one of them folds.
//
// A value long enough to wrap is the ordinary case in this pane — an acceptance
// criterion is a sentence — and folding it back to the left margin puts its
// second half where a key belongs. It goes under the value instead, which is
// what a hanging indent is for.
func fields(rows [][2]string, width int) []string {
	keys := 0

	for _, r := range rows {
		if n := len([]rune(r[0])); n > keys {
			keys = n
		}
	}

	hang := keys + 2
	out := make([]string, 0, len(rows))

	for _, r := range rows {
		folded := wrap(r[1], atLeast(width-hang, 1))

		out = append(out, strings.TrimRight(column(r[0], keys)+"  "+folded[0], " "))

		for _, more := range folded[1:] {
			out = append(out, strings.Repeat(" ", hang)+more)
		}
	}

	return out
}

// claimLine is one branch claiming this issue, and who made it.
func claimLine(claim model.Claim) string {
	who := claim.Claimant
	if who == "" {
		// A claim whose first commit was not looked up is still a claim: what
		// the file says is the claim, and the lookup only names who made it.
		who = "someone"
	}

	line := "claimed by " + who + " on " + claim.Ref
	if claim.Stale {
		line += " — stale"
	}

	return line
}

// wrapAll folds every line to the pane's width. Wrapping rather than
// truncating, because the fields in this pane are sentences — an acceptance
// criterion with its second half cut off is worse than no acceptance criterion.
func wrapAll(in []string, width int) []string {
	out := make([]string, 0, len(in))

	for _, line := range in {
		out = append(out, wrap(line, width)...)
	}

	return out
}

// wrap folds one line at the last space that fits, and mid-word when no space
// does.
//
// It cuts the line rather than rebuilding it out of its words, because the
// spacing inside a line is doing work: rebuilding collapses the two spaces that
// hold a key away from its value into one, so the first fold of a pane quietly
// unaligns every column in it.
func wrap(line string, width int) []string {
	var out []string

	for width >= 1 && visible(line) > width {
		cut := breakAt(line, width)

		out = append(out, strings.TrimRight(line[:cut], " "))
		line = strings.TrimLeft(line[cut:], " ")
	}

	return append(out, line)
}

// breakAt is where to fold a line: the last space that leaves something on both
// sides of it, and the width itself when there is none.
//
// It is only ever called on a line that does not fit, so the width is always
// reached: a line whose characters ran out before it would have fitted.
func breakAt(line string, width int) int {
	last, seen, at := 0, 0, len(line)
	inEscape := false

	for i, r := range line {
		if seen == width {
			at = i

			break
		}

		switch {
		case r == escape:
			inEscape = true
		case inEscape:
			// An escape sequence is not on the screen, so it is not a column
			// the fold has to leave room for. Counting it would fold every
			// coloured line early, and every line is coloured on a terminal.
			inEscape = r != 'm'
		default:
			if r == ' ' && seen > 0 {
				last = i
			}

			seen++
		}
	}

	if last > 0 {
		return last
	}

	return at
}
