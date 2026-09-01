package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dgorshkov/isu/internal/model"
)

// The chrome. Six lines whatever the terminal is: two of header and a rule,
// then a rule, a line for whatever the last action said, and the key hints.
//
// The message line is kept even when there is nothing to say, because a footer
// that grows a line when an action reports moves the whole list up by one under
// somebody's cursor — and the one moment a person is looking hardest at the
// screen is the moment after they pressed `c`.
const (
	headerLines = 3
	footerLines = 3
	chromeLines = headerLines + footerLines
)

// gutter separates the list from the detail pane.
const gutter = " │ "

// listShare is how much of the width the list gets, in percent. The detail pane
// gets the rest, because it is the pane holding sentences.
const listShare = 45

// listWidth is how wide the list pane is drawn.
func (m Model) listWidth() int {
	return atLeast(m.width*listShare/100, 1)
}

// detailWidth is what is left after the list and the gutter.
func (m Model) detailWidth() int {
	return atLeast(m.width-m.listWidth()-len([]rune(gutter)), 1)
}

// bodyHeight is the room the two panes have between the chrome.
func (m Model) bodyHeight() int {
	return atLeast(m.height-chromeLines, 1)
}

// header is the three lines above the panes: what was read, what it adds up to,
// and a rule.
func (m Model) header() []string {
	left := m.styles.title.Render("isu") + "  " +
		m.styles.title.Render(m.in.Trunk) + "  " + plural(m.total(), "issue")

	return []string{
		pad(left, m.styles.dim.Render(m.in.Freshness), m.width),
		m.counts(),
		m.rule(),
	}
}

// counts is the status tally, in the precedence order of PLAN.md's table and
// naming only the statuses something matched — which is the same rule `isu
// board` renders its groups by, so the two cannot disagree about what is on the
// board.
func (m Model) counts() string {
	if m.total() == 0 {
		return m.styles.dim.Render("nothing on any ref isu can see")
	}

	parts := make([]string, 0, len(m.in.Groups))

	for _, group := range m.in.Groups {
		parts = append(parts,
			m.styles.forStatus(group.Status).Render(string(group.Status))+
				" "+strconv.Itoa(len(group.Items)))
	}

	return strings.Join(parts, " · ")
}

// total is how many issues the board holds, which is what the header counts.
func (m Model) total() int {
	n := 0
	for _, group := range m.in.Groups {
		n += len(group.Items)
	}

	return n
}

// footer is the rule, the last thing an action said, and the key map.
func (m Model) footer() []string {
	return []string{m.rule(), m.message(), m.hints()}
}

// message is the filter line, or what the last action reported. Nothing to say
// is an empty line rather than a missing one — see chromeLines.
func (m Model) message() string {
	switch {
	case m.filtering:
		return "/" + m.filter + "▌"
	case m.filter != "":
		return m.styles.dim.Render("/" + m.filter + "  ·  esc clears it")
	default:
		return ""
	}
}

// hints is the key map, which is where a person learns it. PLAN.md M6-S1 names
// seven of these and the eighth is the fold M6-S3's navigation needs.
//
// While the filter line is open the map is a different one, because every
// printable key is a character in the needle and offering `c claim` there would
// be offering something that does not happen.
func (m Model) hints() string {
	keys := []string{
		"enter open", "c claim", "n new", "r ready",
		"g branch", "/ filter", "h fold", "q quit",
	}

	switch {
	case m.filtering:
		keys = []string{"type to narrow", "↑↓ move", "enter accepts", "esc clears"}
	case m.focus == onDetail:
		keys = []string{"↑↓ scroll", "esc back to the list", "q quit"}
	}

	return m.styles.dim.Render(strings.Join(keys, " · "))
}

func (m Model) rule() string { return m.styles.dim.Render(strings.Repeat("─", m.width)) }

// pad puts one string at each end of a line, and drops the right-hand one when
// the line is too narrow to hold both — a freshness line that pushes the trunk
// name off the screen is worse than no freshness line.
func pad(left, right string, width int) string {
	l, r := visible(left), visible(right)
	if l+r+2 > width {
		return left
	}

	return left + strings.Repeat(" ", width-l-r) + right
}

// visible is how many columns a styled string occupies. lipgloss writes escape
// sequences that a terminal does not draw, and counting them would push every
// right-hand column off the screen the moment colour was switched on.
func visible(s string) int {
	n, inEscape := 0, false

	for _, r := range s {
		switch {
		case r == escape:
			inEscape = true
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		default:
			n++
		}
	}

	return n
}

// escape opens an ANSI sequence.
const escape = '\x1b'

// plural counts things in a sentence rather than in a log line, the way
// internal/cli does.
func plural(n int, thing string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, thing)
	}

	return fmt.Sprintf("%d %ss", n, thing)
}

// statusOf is what to call an item's status on screen, with the annotations
// that survive the precedence table.
func statusOf(item *model.Item) string {
	line := string(item.Status)

	var notes []string

	if item.Reopened && item.Status != model.StatusReopened {
		notes = append(notes, "reopened")
	}

	if item.Contended() {
		notes = append(notes, "contended")
	}

	if item.Stale() {
		notes = append(notes, "stale")
	}

	if item.Broken != nil {
		notes = append(notes, "unreadable")
	}

	if len(notes) > 0 {
		line += " (" + strings.Join(notes, ", ") + ")"
	}

	return line
}
