package ui

import (
	"strings"

	"github.com/dgorshkov/isu/internal/model"
)

// The detail pane answers "can I start this?" without leaving the interface,
// which is PLAN.md M6-S4's whole sentence for it. Everything it needs about
// the issue itself is on the item; everything it needs about another issue —
// what a blocker is called and whether it has finished — comes from the board
// the caller derived, because a pane that resolved ids itself would be a pane
// that loaded a repository.
func (m Model) detail(width, height int) []string {
	item := m.selected()
	if item == nil {
		return []string{m.styles.dim.Render("nothing selected")}
	}

	out := []string{
		m.styles.title.Render(item.ID) + "  " + titleOf(item),
		"",
	}

	out = append(out, fields(m.fieldRows(item))...)

	for _, claim := range item.Claims {
		out = append(out, "", m.styles.dim.Render(claimLine(claim)))
	}

	return wrapAll(out, width, height)
}

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

	for n, ref := range item.Elsewhere {
		rows = append(rows, [2]string{key("branches", n), ref})
	}

	if item.Broken != nil {
		rows = append(rows, [2]string{"broken", item.Broken.Err.Error()})
	}

	return rows
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

// fields lays the key/value block out so the values line up.
func fields(rows [][2]string) []string {
	width := 0

	for _, r := range rows {
		if n := len([]rune(r[0])); n > width {
			width = n
		}
	}

	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, strings.TrimRight(column(r[0], width)+"  "+r[1], " "))
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

// wrapAll folds every line to the pane's width and cuts the result to its
// height. Wrapping rather than truncating, because the fields in this pane are
// sentences — an acceptance criterion with its second half cut off is worse
// than no acceptance criterion.
func wrapAll(in []string, width, height int) []string {
	out := make([]string, 0, height)

	for _, line := range in {
		for _, folded := range wrap(line, width) {
			out = append(out, folded)
			if len(out) == height {
				return out
			}
		}
	}

	return out
}

// wrap folds one line at the last space that fits, and mid-word when no space
// does.
func wrap(line string, width int) []string {
	if visible(line) <= width || width < 1 {
		return []string{line}
	}

	var out []string

	for _, word := range strings.Fields(line) {
		switch {
		case len(out) == 0:
			out = append(out, word)
		case visible(out[len(out)-1])+1+visible(word) <= width:
			out[len(out)-1] += " " + word
		default:
			out = append(out, word)
		}
	}

	folded := make([]string, 0, len(out))
	for _, l := range out {
		folded = append(folded, truncate(l, width))
	}

	return folded
}
