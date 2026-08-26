// Package issue is isu's on-disk format: the frontmatter parser, the Issue
// type and the rules that validate it, id generation, and reading and writing
// an issue folder.
//
// Frontmatter is parsed by hand rather than by a YAML library on purpose. It
// is a flat block of key/value lines, and the guarantee that matters most —
// writing an unmodified issue back produces a zero-length diff — is far easier
// to hold when the parser keeps every line it read than when a serialiser is
// free to re-quote, re-order and re-indent. `.isu.yml` is real YAML and gets a
// real parser; this is not.
package issue

import (
	"fmt"
	"strings"
)

// delimiter opens and closes the frontmatter block.
const delimiter = "---"

// ParseError is every failure Parse can report. It carries the line the
// problem is on, because "invalid frontmatter" about a file with forty lines
// in it is not a message anybody can act on.
type ParseError struct {
	// Line is 1-based, counted the way an editor counts.
	Line int
	// Msg says what is wrong, without repeating the line number.
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

// rawLine is one line of the file: its text, and the terminator that followed
// it. Keeping the two apart is what lets a CRLF file stay a CRLF file and a
// file with no final newline stay one.
type rawLine struct {
	text string
	eol  string
}

func (l rawLine) String() string { return l.text + l.eol }

// fmLine is one line inside the frontmatter block. A blank line carries no
// key, and is kept anyway: the round-trip promise is about bytes, not about
// the keys we happen to understand.
type fmLine struct {
	rawLine

	key   string
	value string
}

// Document is a parsed frontmatter file: the delimited key/value block, and
// the markdown body below it.
//
// It keeps every line it read verbatim, so a document that is parsed and
// written back is byte-identical to the one that went in — including unknown
// keys, blank lines, odd spacing, CRLF terminators and a missing final
// newline. Set rewrites one line; everything else is untouched.
type Document struct {
	// Body is everything after the closing delimiter, verbatim.
	Body string

	start rawLine
	lines []fmLine
	end   rawLine
	index map[string]int
}

// NewDocument returns an empty document with LF terminators, for an issue that
// is being created rather than read.
func NewDocument() *Document {
	return &Document{
		start: rawLine{text: delimiter, eol: "\n"},
		end:   rawLine{text: delimiter, eol: "\n"},
		index: map[string]int{},
	}
}

// Parse reads a frontmatter document. It never panics: every malformed input
// comes back as a *ParseError naming the line.
func Parse(data []byte) (*Document, error) {
	raw := splitLines(string(data))
	if len(raw) == 0 {
		return nil, &ParseError{Line: 1, Msg: "the file is empty"}
	}
	if strings.TrimSpace(raw[0].text) != delimiter {
		return nil, &ParseError{
			Line: 1,
			Msg:  fmt.Sprintf("the file must open with %s", delimiter),
		}
	}

	d := &Document{start: raw[0], index: map[string]int{}}

	for i := 1; i < len(raw); i++ {
		// Frontmatter lines land in d.lines in the order they were read, so
		// d.lines[j] is raw[j+1] and a line number is its index plus two.
		lineNo := i + 1

		if strings.TrimSpace(raw[i].text) == delimiter {
			d.end = raw[i]
			d.Body = join(raw[i+1:])

			return d, nil
		}

		if strings.TrimSpace(raw[i].text) == "" {
			d.lines = append(d.lines, fmLine{rawLine: raw[i]})
			continue
		}

		key, value, err := splitPair(raw[i].text, lineNo)
		if err != nil {
			return nil, err
		}
		if prev, dup := d.index[key]; dup {
			return nil, &ParseError{
				Line: lineNo,
				Msg: fmt.Sprintf("duplicate key %q, already set on line %d",
					key, prev+2),
			}
		}

		d.index[key] = len(d.lines)
		d.lines = append(d.lines, fmLine{rawLine: raw[i], key: key, value: value})
	}

	return nil, &ParseError{
		Line: len(raw),
		Msg:  fmt.Sprintf("the frontmatter opened on line 1 is never closed by %s", delimiter),
	}
}

// Get returns a key's value and whether it was present. The value is trimmed
// of surrounding whitespace; the line it came from is not.
func (d *Document) Get(key string) (string, bool) {
	i, ok := d.index[key]
	if !ok {
		return "", false
	}

	return d.lines[i].value, true
}

// Has reports whether the key is present, including when its value is empty.
func (d *Document) Has(key string) bool {
	_, ok := d.index[key]

	return ok
}

// Keys lists the keys in the order the file declares them.
func (d *Document) Keys() []string {
	keys := make([]string, 0, len(d.index))
	for _, l := range d.lines {
		if l.key != "" {
			keys = append(keys, l.key)
		}
	}

	return keys
}

// Set writes a key, rewriting its line in place if it is already there and
// appending it to the end of the block if it is not. Every other line is left
// exactly as it was read.
//
// Values are one line: this format has no folding and no quoting, so a value
// carrying a line break would produce a file that does not parse back. Any
// carriage return or newline in value becomes a space.
func (d *Document) Set(key, value string) {
	value = oneLine(value)

	if i, ok := d.index[key]; ok {
		d.lines[i].text = key + ": " + value
		d.lines[i].value = value

		return
	}

	d.index[key] = len(d.lines)
	d.lines = append(d.lines, fmLine{
		rawLine: rawLine{text: key + ": " + value, eol: d.start.eol},
		key:     key,
		value:   value,
	})
}

// Unset removes a key. Removing one that is not there does nothing.
func (d *Document) Unset(key string) {
	i, ok := d.index[key]
	if !ok {
		return
	}

	d.lines = append(d.lines[:i], d.lines[i+1:]...)
	d.index = make(map[string]int, len(d.lines))
	for j, l := range d.lines {
		if l.key != "" {
			d.index[l.key] = j
		}
	}
}

// String renders the document back to the text it was parsed from.
func (d *Document) String() string {
	var b strings.Builder

	b.WriteString(d.start.String())
	for _, l := range d.lines {
		b.WriteString(l.String())
	}
	b.WriteString(d.end.String())
	b.WriteString(d.Body)

	return b.String()
}

// Bytes renders the document back to the bytes it was parsed from.
func (d *Document) Bytes() []byte { return []byte(d.String()) }

// splitPair breaks one frontmatter line into its key and value at the first
// colon. A value may contain as many more colons as it likes.
func splitPair(text string, lineNo int) (key, value string, err error) {
	i := strings.IndexByte(text, ':')
	if i < 0 {
		return "", "", &ParseError{
			Line: lineNo,
			Msg:  fmt.Sprintf("expected `key: value`, found %q", strings.TrimSpace(text)),
		}
	}

	key = strings.TrimSpace(text[:i])
	if key == "" {
		return "", "", &ParseError{Line: lineNo, Msg: "the key is empty"}
	}
	if !validKey(key) {
		return "", "", &ParseError{
			Line: lineNo,
			Msg:  fmt.Sprintf("%q is not a key: keys are letters, digits, - and _", key),
		}
	}

	return key, strings.TrimSpace(text[i+1:]), nil
}

// validKey reports whether a frontmatter key is one this format allows.
func validKey(key string) bool {
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}

	return true
}

// splitLines cuts text into lines, keeping each line's terminator beside it
// rather than throwing it away. A final line with no terminator comes back
// with an empty one, which is how it is written out again.
func splitLines(text string) []rawLine {
	var lines []rawLine

	for text != "" {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			lines = append(lines, rawLine{text: text})
			break
		}

		line, eol := text[:i], "\n"
		if strings.HasSuffix(line, "\r") {
			line, eol = line[:len(line)-1], "\r\n"
		}

		lines = append(lines, rawLine{text: line, eol: eol})
		text = text[i+1:]
	}

	return lines
}

// join is the inverse of splitLines.
func join(lines []rawLine) string {
	var b strings.Builder

	for _, l := range lines {
		b.WriteString(l.String())
	}

	return b.String()
}

// oneLine flattens the line breaks out of a value.
var lineBreaks = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

func oneLine(s string) string { return lineBreaks.Replace(s) }
