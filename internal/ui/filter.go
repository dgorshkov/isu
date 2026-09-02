package ui

import (
	"strings"

	"github.com/dgorshkov/isu/internal/model"
)

// The filter is a literal substring across the five fields somebody actually
// looks an issue up by, and it is deliberately not a pattern.
//
// A tracker's search box is typed into by people looking for `ISU-4jsxyw`, for
// `login`, and — because this one tracks its own construction — for issues
// whose titles contain `*`, `[` and `.`. A regular expression turns the first
// two into a lottery and the third into an error message, so this is
// strings.Contains over a lowercased haystack and nothing cleverer.

// searchable is what a filter is matched against: the five fields somebody
// looks an issue up by, lowercased and joined, once per issue.
//
// Once, rather than on every keystroke, and that is the difference between
// meeting M6-S3's frame budget and missing it. Lowercasing five fields per
// issue per keypress is twenty-five thousand allocations a keypress on a
// five-thousand-issue board; doing it at startup is one pass over a board that
// has just been read out of git, and the fields cannot change underneath it
// because nothing here loads anything.
func searchable(item *model.Item) string {
	parts := []string{item.ID, string(item.Status)}

	if item.Issue != nil {
		parts = append(parts, item.Issue.Title, string(item.Issue.Type), item.Issue.Owner)
	}

	return strings.ToLower(strings.Join(parts, "\n"))
}

// index builds the haystacks for every issue the interface was handed.
func index(groups []Group, ready []*model.Item) map[string]string {
	out := map[string]string{}

	add := func(items []*model.Item) {
		for _, item := range items {
			if _, done := out[item.ID]; !done {
				out[item.ID] = searchable(item)
			}
		}
	}

	for _, group := range groups {
		add(group.Items)
	}

	add(ready)

	return out
}

// matches reports whether an issue is one the needle is looking for.
func (m Model) matches(item *model.Item) bool {
	return strings.Contains(m.search[item.ID], m.filter)
}

// filtered is the groups this filter leaves, dropping the ones nothing in them
// matched — the same rule the board drops an empty status by.
func (m Model) filtered() []Group {
	if m.filter == "" {
		return m.in.Groups
	}

	out := make([]Group, 0, len(m.in.Groups))

	for _, group := range m.in.Groups {
		kept := make([]*model.Item, 0, len(group.Items))

		for _, item := range group.Items {
			if m.matches(item) {
				kept = append(kept, item)
			}
		}

		if len(kept) > 0 {
			out = append(out, Group{Status: group.Status, Items: kept})
		}
	}

	return out
}

// typeInto is one keypress while the filter line is open.
//
// Every printable key is a character in the needle rather than a command: a `q`
// that quit the program half way through typing "queue" would make the filter
// unusable, and a `/` that had to be escaped would be worse.
func (m Model) typeInto(name string, runes []rune) (Model, bool) {
	switch name {
	case "esc":
		m.filter = ""
		m.filtering = false
	case "enter":
		// The needle stays; the line closes. What somebody has narrowed to is
		// the list they wanted, and they now want the keys back.
		m.filtering = false
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	default:
		if len(runes) == 0 {
			return m, false
		}

		m.filter += strings.ToLower(string(runes))
	}

	m.rebuild()

	return m, true
}

// fold collapses the epic the cursor is on, or the one it is inside.
//
// Folding from inside an epic moves the cursor onto the epic rather than
// leaving it on a row that is about to stop being drawn — losing the cursor is
// the one thing a fold must not do.
func (m Model) fold() Model {
	item := m.selected()
	if item == nil {
		return m
	}

	id := item.ID
	if item.Epic == nil {
		parent := parentOf(item)
		if parent == "" {
			return m
		}

		id = parent
		m.sticky = parent
	}

	m.collapsed[id] = true
	m.rebuild()

	return m
}

// unfold opens the epic the cursor is on.
func (m Model) unfold() Model {
	item := m.selected()
	if item == nil {
		return m
	}

	delete(m.collapsed, item.ID)
	m.rebuild()

	return m
}

// move steps the cursor over the rows it may land on, which is every issue and
// no heading. A step that runs off either end stops at the last row it could
// have landed on.
func (m Model) move(delta int) Model {
	if delta == 0 {
		return m
	}

	step := 1
	if delta < 0 {
		step = -1
	}

	for range abs(delta) {
		m = m.step(step)
	}

	m.sticky = m.selectedID()
	m.detailTop = 0
	m.scroll()

	return m
}

func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}

// step moves the cursor by one selectable row in one direction.
func (m Model) step(delta int) Model {
	for i := m.cursor + delta; i >= 0 && i < len(m.rows); i += delta {
		if m.rows[i].selectable() {
			m.cursor = i

			break
		}
	}

	return m
}
