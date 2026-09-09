package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// theme is how output is coloured, which is a decision about the terminal
// rather than about the data.
//
// Colour is off unless every one of three things is true: the stream is a
// character device, the terminal is not `dumb`, and NO_COLOR is unset. A tool
// that writes escape sequences into a pipe produces a file nobody can grep, and
// every test in this package writes into a buffer, so the tests are colourless
// for the same reason a redirect is.
type theme struct {
	color bool
}

// ANSI is written by hand here rather than pulled in from a styling library.
// lipgloss is in the plan's allowlist for the TUI, where a layout engine earns
// its place; a board that dims one column does not.
const (
	ansiReset = "\x1b[0m"
	ansiDim   = "\x1b[2m"
	ansiBold  = "\x1b[1m"
)

func (t theme) wrap(code, s string) string {
	if !t.color || s == "" {
		return s
	}

	return code + s + ansiReset
}

func (t theme) dim(s string) string  { return t.wrap(ansiDim, s) }
func (t theme) bold(s string) string { return t.wrap(ansiBold, s) }

// themeFor decides whether this invocation may colour its output.
func (a *app) themeFor(w io.Writer) theme {
	if a.noColor || a.env.getenv("NO_COLOR") != "" {
		return theme{}
	}

	if term := a.env.getenv("TERM"); term == "" || term == "dumb" {
		return theme{}
	}

	return theme{color: isCharDevice(w)}
}

// isCharDevice is the cheap half of asking whether something is a terminal: a
// pipe and a file are not character devices and a terminal is. It is the same
// heuristic every colouring tool uses, and it costs no dependency.
func isCharDevice(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

// age renders a duration the way a person says one. Precision past the leading
// unit is noise on a board: nobody triages by minutes in the third column.
func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

// freshnessLine is the sentence `isu board` and `isu show` both print.
func freshnessLine(f Freshness, warnAfter time.Duration) string {
	if !f.Remote {
		return "no remote refs"
	}

	line := "remote refs " + age(time.Duration(f.AgeSeconds)*time.Second)
	if f.Warn {
		line += fmt.Sprintf(" — older than %s, run with --fetch", hours(warnAfter))
	}

	return line
}

// hours spells a whole number of hours, which is the only shape
// fetch_warn_hours can take.
func hours(d time.Duration) string {
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// columns lays rows out so that every column is as wide as its widest cell.
//
// The last cell of a row is not padded: a trailing run of spaces is invisible
// on a terminal and very visible in a golden file, and this package has a lot
// of golden files.
func columns(rows [][]string, indent string) []string {
	widths := []int{}

	for _, row := range rows {
		for i, cell := range row {
			for len(widths) <= i {
				widths = append(widths, 0)
			}

			if n := len([]rune(cell)); n > widths[i] {
				widths[i] = n
			}
		}
	}

	lines := make([]string, 0, len(rows))

	for _, row := range rows {
		var b strings.Builder

		b.WriteString(indent)

		for i, cell := range row {
			if i == len(row)-1 {
				b.WriteString(cell)

				break
			}

			b.WriteString(cell)
			b.WriteString(strings.Repeat(" ", widths[i]-len([]rune(cell))+2))
		}

		lines = append(lines, strings.TrimRight(b.String(), " "))
	}

	return lines
}
