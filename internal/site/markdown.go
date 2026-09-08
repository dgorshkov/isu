package site

import (
	"fmt"
	"html"
	"strings"
	"unicode"
)

// A small markdown renderer, for the documents in docs/ and for nothing else.
//
// PLAN.md's out-of-scope list drops the local web UI and says dropping it "also
// drops the markdown-to-HTML pipeline, the HTML sanitiser and the XSS surface
// that came with them". That is a rule about *issue bodies*, which arrive from
// an importer and from strangers; the docs are written in this repository and
// reviewed in the same pull request as the code. The distinction is worth
// keeping honest, so this renderer has no way to become the other thing:
//
//   - it never passes raw HTML through — a `<script>` in the source is escaped
//     and rendered as the six characters somebody typed — so there is nothing
//     for a sanitiser to do and no configuration in which there would be;
//   - it renders a fixed subset and reports a line it does not understand,
//     rather than degrading to "print it as text";
//   - it is never handed an issue body. The samples on the site come out of
//     isu's own renderer and are escaped into a <pre>.
//
// The subset is: ATX headings, paragraphs, `- ` and `1. ` lists, pipe tables,
// fenced code, blockquotes, thematic breaks, and inline code, `*emphasis*`,
// `**strong**` and `[links](…)`.

// Doc is one rendered markdown document.
type Doc struct {
	// Title is the `# ` heading, which every document in docs/ opens with.
	Title string
	// Lede is the first paragraph, used as the page description.
	Lede string
	// HTML is the body, without the title.
	HTML string
	// Headings are the `## ` headings, in order, for the page's own contents
	// list.
	Headings []Heading
}

// Heading is one `## ` heading and the anchor it can be linked to.
type Heading struct {
	Text string
	ID   string
}

// Markdown renders a document. The error names the first line the subset above
// does not cover, so an unsupported construct is a build failure rather than a
// page with a stray asterisk in it.
func Markdown(doc string) (Doc, error) {
	r := &renderer{lines: strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n")}
	if err := r.run(); err != nil {
		return Doc{}, err
	}

	return Doc{
		Title:    r.title,
		Lede:     r.lede,
		HTML:     strings.Join(r.out, "\n") + "\n",
		Headings: r.headings,
	}, nil
}

type renderer struct {
	lines    []string
	at       int
	out      []string
	title    string
	lede     string
	headings []Heading
}

func (r *renderer) run() error {
	for r.at < len(r.lines) {
		line := r.lines[r.at]

		switch {
		case strings.TrimSpace(line) == "":
			r.at++
		case strings.HasPrefix(line, "```"):
			r.code()
		case strings.HasPrefix(line, "#"):
			if err := r.heading(); err != nil {
				return err
			}
		case strings.TrimSpace(line) == "---":
			r.out = append(r.out, "<hr>")
			r.at++
		case strings.HasPrefix(line, "> "):
			r.quote()
		case strings.HasPrefix(line, "- "):
			r.list("ul", "- ")
		case orderedPrefix(line) != "":
			r.list("ol", orderedPrefix(line))
		case strings.HasPrefix(line, "|"):
			if err := r.table(); err != nil {
				return err
			}
		default:
			r.paragraph()
		}
	}

	if r.title == "" {
		return fmt.Errorf("a document with no `# ` heading has no title")
	}

	return nil
}

// heading renders one ATX heading and remembers the `##` ones for the contents
// list beside the page.
func (r *renderer) heading() error {
	line := r.lines[r.at]
	r.at++

	hashes := 0
	for hashes < len(line) && line[hashes] == '#' {
		hashes++
	}

	if hashes > 4 || hashes >= len(line) || line[hashes] != ' ' {
		return fmt.Errorf("line %d: %q is not a heading this renderer knows", r.at, line)
	}

	text := strings.TrimSpace(line[hashes+1:])

	if hashes == 1 {
		if r.title != "" {
			return fmt.Errorf("line %d: a second `# ` heading; a page has one title", r.at)
		}

		r.title = text

		return nil
	}

	id := Slug(text)
	if hashes == 2 {
		r.headings = append(r.headings, Heading{Text: text, ID: id})
	}

	r.out = append(r.out, fmt.Sprintf(`<h%d id="%s">%s</h%d>`, hashes, id, Inline(text), hashes))

	return nil
}

// paragraph runs to the next blank line or block opener.
func (r *renderer) paragraph() {
	var para []string

	for r.at < len(r.lines) {
		line := r.lines[r.at]
		if strings.TrimSpace(line) == "" || opensBlock(line) {
			break
		}

		para = append(para, strings.TrimSpace(line))
		r.at++
	}

	text := strings.Join(para, " ")
	if r.lede == "" {
		r.lede = plain(text)
	}

	r.out = append(r.out, "<p>"+Inline(text)+"</p>")
}

func (r *renderer) list(tag, prefix string) {
	items := []string{}

	for r.at < len(r.lines) {
		line := r.lines[r.at]

		want := prefix
		if tag == "ol" {
			want = orderedPrefix(line)
		}

		if want == "" || !strings.HasPrefix(line, want) {
			break
		}

		items = append(items, "<li>"+Inline(strings.TrimSpace(line[len(want):]))+"</li>")
		r.at++
	}

	r.out = append(r.out, "<"+tag+">")
	r.out = append(r.out, items...)
	r.out = append(r.out, "</"+tag+">")
}

func (r *renderer) quote() {
	var lines []string

	for r.at < len(r.lines) && strings.HasPrefix(r.lines[r.at], "> ") {
		lines = append(lines, strings.TrimSpace(strings.TrimPrefix(r.lines[r.at], "> ")))
		r.at++
	}

	r.out = append(r.out, "<blockquote><p>"+Inline(strings.Join(lines, " "))+"</p></blockquote>")
}

// code renders a fenced block. Wide output is the reason every one of them
// sits in its own scrolling box: PLAN.md M8-S3 asks for no horizontal scroll on
// the page at 360 px, and a board is 90 columns wide.
//
// **A block that ran and a block that did not must not look the same.** The
// dark ground on this site means "these are the bytes the binary wrote" — the
// whole of M8's governing constraint is that claim — and an illustrative
// snippet with a placeholder id in it had been borrowing that ground for free.
// A reader who pastes one and watches it fail has been told something false by
// a page whose entire argument is that it never does. So a ```console fence,
// which internal/site executes, keeps the dark ground; every other fence gets a
// light one it cannot be confused with.
func (r *renderer) code() {
	line := strings.TrimSpace(r.lines[r.at])
	marker := fenceMarker(line)

	kind := "sketch"
	if isConsole(strings.TrimPrefix(line, marker)) {
		kind = "ran"
	}

	r.at++

	var body []string

	for r.at < len(r.lines) && strings.TrimSpace(r.lines[r.at]) != marker {
		body = append(body, html.EscapeString(r.lines[r.at]))
		r.at++
	}

	r.at++

	r.out = append(r.out, `<div class="scroller `+kind+`"><pre><code>`+
		strings.Join(body, "\n")+"</code></pre></div>")
}

// table renders a pipe table. The header row and the `|---|` row beneath it are
// both required, because a table without a header is a table a screen reader
// reads as nine hundred unlabelled cells.
func (r *renderer) table() error {
	head := cells(r.lines[r.at])

	if r.at+1 >= len(r.lines) || !isRule(r.lines[r.at+1]) {
		return fmt.Errorf("line %d: a table needs a |---| row under its header", r.at+1)
	}

	r.at += 2

	out := []string{`<div class="scroller"><table>`, "<thead><tr>"}
	for _, cell := range head {
		out = append(out, "<th>"+Inline(cell)+"</th>")
	}

	out = append(out, "</tr></thead>", "<tbody>")

	for r.at < len(r.lines) && strings.HasPrefix(r.lines[r.at], "|") {
		out = append(out, "<tr>")
		for _, cell := range cells(r.lines[r.at]) {
			out = append(out, "<td>"+Inline(cell)+"</td>")
		}

		out = append(out, "</tr>")
		r.at++
	}

	r.out = append(r.out, append(out, "</tbody></table></div>")...)

	return nil
}

func cells(line string) []string {
	fields := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	for i, field := range fields {
		fields[i] = strings.TrimSpace(field)
	}

	return fields
}

func isRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return false
	}

	return strings.Trim(trimmed, "|-: \t") == ""
}

// opensBlock reports whether a line ends the paragraph before it.
func opensBlock(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") ||
		strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "> ") ||
		strings.HasPrefix(line, "|") || orderedPrefix(line) != "" ||
		strings.TrimSpace(line) == "---"
}

// orderedPrefix is the `12. ` an ordered list item opens with, or empty.
func orderedPrefix(line string) string {
	digits := 0
	for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
		digits++
	}

	if digits == 0 || !strings.HasPrefix(line[digits:], ". ") {
		return ""
	}

	return line[:digits+2]
}

// Inline renders the inline subset. Everything that is not markup this
// function emits is escaped, so nothing in a source document can become markup
// by accident or otherwise.
func Inline(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '`':
			end := strings.IndexByte(s[i+1:], '`')
			if end < 0 {
				b.WriteString(html.EscapeString(s[i : i+1]))

				continue
			}

			b.WriteString("<code>" + html.EscapeString(s[i+1:i+1+end]) + "</code>")
			i += end + 1
		case strings.HasPrefix(s[i:], "**"):
			end := strings.Index(s[i+2:], "**")
			if end < 0 {
				b.WriteString("**")
				i++

				continue
			}

			b.WriteString("<strong>" + Inline(s[i+2:i+2+end]) + "</strong>")
			i += end + 3
		case s[i] == '*':
			end := strings.IndexByte(s[i+1:], '*')
			if end < 0 {
				b.WriteString("*")

				continue
			}

			b.WriteString("<em>" + Inline(s[i+1:i+1+end]) + "</em>")
			i += end + 1
		case s[i] == '[':
			text, href, width, ok := readLink(s[i:])
			if !ok {
				b.WriteString("[")

				continue
			}

			b.WriteString(`<a href="` + html.EscapeString(href) + `">` + Inline(text) + "</a>")
			i += width - 1
		default:
			b.WriteString(html.EscapeString(s[i : i+1]))
		}
	}

	return b.String()
}

// readLink reads `[text](href)` and says how many bytes it spanned.
func readLink(s string) (text, href string, width int, ok bool) {
	closeText := strings.IndexByte(s, ']')
	if closeText < 0 || !strings.HasPrefix(s[closeText+1:], "(") {
		return "", "", 0, false
	}

	closeHref := strings.IndexByte(s[closeText+2:], ')')
	if closeHref < 0 {
		return "", "", 0, false
	}

	return s[1:closeText], s[closeText+2 : closeText+2+closeHref], closeText + 3 + closeHref, true
}

// plain is inline markdown with the markup taken off, for a description that
// goes in a meta tag rather than into the page.
func plain(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "`", "")

	for {
		open := strings.IndexByte(s, '[')
		if open < 0 {
			return s
		}

		text, _, width, ok := readLink(s[open:])
		if !ok {
			return s
		}

		s = s[:open] + text + s[open+width:]
	}
}

// Slug is a heading's anchor: lowercase, words joined by hyphens, and nothing
// that would need escaping in a URL.
func Slug(s string) string {
	var b strings.Builder

	dash := false

	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}

			dash = false

			b.WriteRune(r)
		default:
			dash = true
		}
	}

	return b.String()
}
