package ui

import (
	"strconv"
	"strings"

	"github.com/dgorshkov/isu/internal/model"
)

// row is one drawn line of the list: a status heading, or an issue under it.
type row struct {
	// heading is the status this row introduces, and is empty on an issue row.
	heading string
	count   int
	item    *model.Item
	// depth is how far the row is indented, which is how an epic's children are
	// shown to belong to it.
	depth int
	// hidden is how many rows this epic's fold is holding back, and is zero on
	// everything that is not a folded epic.
	hidden int
}

// selectable reports whether the cursor may land here. It may not land on a
// heading: a heading is not a thing anybody can claim.
func (r row) selectable() bool { return r.item != nil }

// rebuild builds the list from what the interface was handed.
//
// It keeps the cursor on the issue it was on where that issue is still in the
// list, which is what makes `r`, a filter and an action all leave somebody
// where they were rather than at the top.
func (m *Model) rebuild() {
	was := m.sticky
	if was == "" {
		was = m.selectedID()
	}

	m.rows = m.rows[:0]

	if m.ready {
		m.rows = append(m.rows, row{heading: "ready", count: len(m.in.Ready)})

		for _, item := range m.in.Ready {
			m.rows = append(m.rows, row{item: item})
		}
	} else {
		for _, group := range m.filtered() {
			m.rows = append(m.rows, row{heading: string(group.Status), count: len(group.Items)})
			m.rows = arrange(m.rows, group.Items, m.collapsed)
		}
	}

	m.restore(was)
	m.scroll()
}

// arrange draws an epic's children under it.
//
// The order inside a group is the board's, with one thing done to it: a child
// whose epic is in the same group is drawn immediately after that epic, one
// level in. Which group an issue is in is not touched — that is the board's
// answer and PLAN.md M6-S2 asks that the two agree exactly — so a child whose
// epic has finished stands at the top of its own group rather than being drawn
// under an epic three groups away.
//
// Every issue is emitted exactly once. A parent cycle is two epics that are
// each other's ancestors, which is `isu check`'s to report and this function's
// to survive: the sweep at the end draws whatever the walk could not reach,
// rather than following the chain until the stack runs out.
func arrange(rows []row, items []*model.Item, collapsed map[string]bool) []row {
	children := map[string][]*model.Item{}
	here := make(map[string]bool, len(items))

	for _, item := range items {
		here[item.ID] = true
	}

	for _, item := range items {
		if parent := parentOf(item); parent != "" && here[parent] {
			children[parent] = append(children[parent], item)
		}
	}

	drawn := make(map[string]bool, len(items))

	var emit func(*model.Item, int)

	emit = func(item *model.Item, depth int) {
		if drawn[item.ID] {
			return
		}

		drawn[item.ID] = true
		if collapsed[item.ID] {
			// The children are still in the group and still counted; what a
			// fold takes away is the rows, and `j` steps over them because
			// they are not there to step onto.
			rows = append(rows, row{
				item: item, depth: depth, hidden: countUnder(children, item.ID),
			})
			markDrawn(drawn, children, item.ID)

			return
		}

		rows = append(rows, row{item: item, depth: depth})

		for _, child := range children[item.ID] {
			emit(child, depth+1)
		}
	}

	for _, item := range items {
		if parent := parentOf(item); parent == "" || !here[parent] {
			emit(item, 0)
		}
	}

	// Whatever the walk could not reach from a root, which is every issue in a
	// parent cycle. They are drawn at the top level, in the board's order.
	for _, item := range items {
		emit(item, 0)
	}

	return rows
}

// countUnder is how many rows a fold is holding back, which is what its row
// says instead of drawing them.
func countUnder(children map[string][]*model.Item, id string) int {
	n := 0

	for _, child := range children[id] {
		n += 1 + countUnder(children, child.ID)
	}

	return n
}

// markDrawn says a folded epic's children have been dealt with, so that the
// sweep for a parent cycle does not draw them at the top level instead.
func markDrawn(drawn map[string]bool, children map[string][]*model.Item, id string) {
	for _, child := range children[id] {
		if !drawn[child.ID] {
			drawn[child.ID] = true
			markDrawn(drawn, children, child.ID)
		}
	}
}

// parentOf is the epic an issue names, or nothing.
func parentOf(item *model.Item) string {
	if item.Issue == nil {
		return ""
	}

	return item.Issue.Parent
}

// restore puts the cursor back on an issue, or on the first one there is.
func (m *Model) restore(id string) {
	for i, r := range m.rows {
		if r.selectable() && r.item.ID == id {
			m.cursor = i

			return
		}
	}

	m.cursor = m.firstSelectable()
}

// firstSelectable is the first row the cursor may sit on, and the length of the
// list when there is none — an empty list has a cursor that is nowhere, and
// nowhere has to be somewhere.
func (m Model) firstSelectable() int {
	for i, r := range m.rows {
		if r.selectable() {
			return i
		}
	}

	return len(m.rows)
}

// selected is the issue the cursor is on, or nil.
func (m Model) selected() *model.Item {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}

	return m.rows[m.cursor].item
}

// selectedID is the id of the issue the cursor is on, or empty.
func (m Model) selectedID() string {
	if item := m.selected(); item != nil {
		return item.ID
	}

	return ""
}

// scroll keeps the cursor on screen, and keeps the list from scrolling past its
// own end.
func (m *Model) scroll() {
	height := m.bodyHeight()

	if m.cursor < m.top {
		m.top = m.cursor
	}

	if m.cursor >= m.top+height {
		m.top = m.cursor - height + 1
	}

	if ceiling := len(m.rows) - height; m.top > ceiling {
		m.top = ceiling
	}

	if m.top < 0 {
		m.top = 0
	}
}

// list draws the issues.
func (m Model) list(width, height int) []string {
	if len(m.rows) == 0 {
		return []string{m.styles.dim.Render(m.emptyLine())}
	}

	ids := m.idWidth()

	out := make([]string, 0, height)

	for i := m.top; i < len(m.rows) && len(out) < height; i++ {
		out = append(out, m.line(i, ids, width))
	}

	return out
}

// emptyLine is what an empty list says. A blank pane is a bug report waiting to
// be filed; a sentence is an answer.
func (m Model) emptyLine() string {
	if m.filter != "" {
		return "nothing matches " + m.filter
	}

	if m.ready {
		return "nothing is ready: everything open is blocked, claimed or untriaged"
	}

	return "no issues on any ref isu can see"
}

// idWidth is how wide the id column has to be. Ids are permanent and imported
// ones can be any length, so the column is measured rather than assumed.
func (m Model) idWidth() int {
	width := 0

	for _, r := range m.rows {
		if r.selectable() {
			if n := len([]rune(r.item.ID)) + r.depth*indent; n > width {
				width = n
			}
		}
	}

	return width
}

// indent is how far one level of nesting moves a row.
const indent = 2

// line draws one row of the list.
func (m Model) line(i, ids, width int) string {
	r := m.rows[i]

	if !r.selectable() {
		return m.styles.heading.Render(
			m.styles.forStatus(model.Status(r.heading)).Render(r.heading) +
				" (" + strconv.Itoa(r.count) + ")")
	}

	id := strings.Repeat(" ", r.depth*indent) + r.item.ID
	title := titleOf(r.item)

	if r.hidden > 0 {
		// The fold is named beside the title rather than beside the id,
		// because a marker glued to an id is a marker somebody copies with it.
		title += "  (" + strconv.Itoa(r.hidden) + " folded)"
	}

	body := column(id, ids) + "  " + priorityOf(r.item) + "  " + title

	marker := "  "
	if i == m.cursor {
		marker = "▸ "
	}

	body = truncate(body, atLeast(width-len([]rune(marker)), 1))

	if i == m.cursor {
		return marker + m.styles.selected.Render(body)
	}

	return marker + m.styles.forStatus(r.item.Status).Render(body)
}

// column pads a cell to a width, in characters rather than bytes.
func column(s string, width int) string {
	if n := len([]rune(s)); n < width {
		return s + strings.Repeat(" ", width-n)
	}

	return s
}

// titleOf is what to call an issue on screen. An issue whose file will not
// decode still has a row, and the row still has to say something.
func titleOf(item *model.Item) string {
	if item.Issue == nil || item.Issue.Title == "" {
		return "(no title)"
	}

	return item.Issue.Title
}

// priorityOf is the priority column, defaulted the way the schema defaults it.
func priorityOf(item *model.Item) string {
	if item.Issue == nil {
		return "--"
	}

	return string(item.Issue.EffectivePriority())
}

// typeOf is what kind of issue this is, for a pane with room to say.
func typeOf(item *model.Item) string {
	if item.Issue == nil {
		return ""
	}

	return string(item.Issue.Type)
}
