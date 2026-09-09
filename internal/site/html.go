package site

import (
	"fmt"
	"strings"
)

// A very small HTML reader, for the gates below and for nothing else.
//
// It is written here rather than pulled in because PLAN.md §0 fixes the
// dependency allowlist and a parser is not on it — and because the gates need
// less than a parser: a list of elements, their attributes, their text and
// whether they close in the right order. Frontmatter is read by hand in
// internal/issue for the same reason, and this is the same bargain.
//
// It is deliberately strict. Every construct the site's own templates can emit
// is understood, and anything else is an error rather than a guess, because a
// gate that quietly skips what it cannot read is a gate that passes everything.

// voidElements never close, so a stack must not wait for them.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// Element is one tag in a document, with the text it directly contains.
type Element struct {
	// Name is lowercased.
	Name string
	// Attr is its attributes, lowercased names, unescaped values.
	Attr map[string]string
	// Text is everything between this element's start and end tags, with tags
	// removed — which is what "does this link have discernible text" asks.
	Text string
	// Depth is how many open elements enclose it.
	Depth int
}

// Attribute is one attribute, or the empty string.
func (e Element) Attribute(name string) string { return e.Attr[name] }

// ParseHTML reads a document into the elements it contains, in document order,
// and reports the first thing that is not well formed.
func ParseHTML(doc string) ([]Element, error) {
	var (
		out   []Element
		stack []int
		text  strings.Builder
	)

	if !strings.HasPrefix(strings.TrimSpace(doc), "<!DOCTYPE html>") {
		return nil, fmt.Errorf("the document does not open with <!DOCTYPE html>")
	}

	for i := 0; i < len(doc); i++ {
		if doc[i] != '<' {
			text.WriteByte(doc[i])

			continue
		}

		if skip, ok := skippable(doc[i:]); ok {
			i += skip - 1

			continue
		}

		end := tagEnd(doc[i:])
		if end < 0 {
			return nil, fmt.Errorf("a `<` at byte %d never closes", i)
		}

		inner := doc[i+1 : i+end]
		i += end

		// Text belongs to every element still open, which is what makes
		// "discernible text" work for a link wrapping a span.
		for _, at := range stack {
			out[at].Text += text.String()
		}

		text.Reset()

		if strings.HasPrefix(inner, "/") {
			var err error
			if stack, err = closeTag(out, stack, strings.ToLower(inner[1:])); err != nil {
				return nil, err
			}

			continue
		}

		element, err := openTag(inner, len(stack))
		if err != nil {
			return nil, err
		}

		out = append(out, element)

		if !voidElements[element.Name] && !strings.HasSuffix(strings.TrimSpace(inner), "/") {
			stack = append(stack, len(out)-1)
		}
	}

	if len(stack) > 0 {
		return nil, fmt.Errorf("<%s> is never closed", out[stack[len(stack)-1]].Name)
	}

	return out, nil
}

// skippable is a doctype or a comment, and how many bytes it spans.
func skippable(s string) (int, bool) {
	if strings.HasPrefix(s, "<!--") {
		end := strings.Index(s, "-->")
		if end < 0 {
			return len(s), true
		}

		return end + 3, true
	}

	if strings.HasPrefix(s, "<!") {
		end := strings.IndexByte(s, '>')
		if end < 0 {
			return len(s), true
		}

		return end + 1, true
	}

	return 0, false
}

// tagEnd is the offset of the `>` that closes a tag, ignoring one inside a
// quoted attribute value.
func tagEnd(s string) int {
	quote := byte(0)

	for i := 1; i < len(s); i++ {
		switch {
		case quote != 0:
			if s[i] == quote {
				quote = 0
			}
		case s[i] == '"' || s[i] == '\'':
			quote = s[i]
		case s[i] == '>':
			return i
		}
	}

	return -1
}

// closeTag pops the stack, refusing a close that does not match what is open.
func closeTag(out []Element, stack []int, name string) ([]int, error) {
	if len(stack) == 0 {
		return nil, fmt.Errorf("</%s> closes nothing", name)
	}

	open := out[stack[len(stack)-1]].Name
	if open != name {
		return nil, fmt.Errorf("</%s> closes <%s>", name, open)
	}

	return stack[:len(stack)-1], nil
}

// openTag reads a start tag's name and attributes.
func openTag(inner string, depth int) (Element, error) {
	inner = strings.TrimSuffix(strings.TrimSpace(inner), "/")

	name, rest, _ := strings.Cut(strings.TrimSpace(inner), " ")

	element := Element{Name: strings.ToLower(name), Attr: map[string]string{}, Depth: depth}
	if element.Name == "" {
		return Element{}, fmt.Errorf("a tag with no name")
	}

	for rest = strings.TrimSpace(rest); rest != ""; rest = strings.TrimSpace(rest) {
		key, after, found := strings.Cut(rest, "=")
		if !found {
			element.Attr[strings.ToLower(strings.TrimSpace(rest))] = ""

			break
		}

		key = strings.ToLower(strings.TrimSpace(key))

		if !strings.HasPrefix(after, `"`) {
			return Element{}, fmt.Errorf("<%s %s=…> is not a quoted attribute value",
				element.Name, key)
		}

		value, remainder, closed := strings.Cut(after[1:], `"`)
		if !closed {
			return Element{}, fmt.Errorf("<%s %s=…> has an unclosed attribute value",
				element.Name, key)
		}

		element.Attr[key] = unescape(value)
		rest = remainder
	}

	return element, nil
}

// unescape reverses the entities html/template writes, which is enough to
// compare an href with a path.
func unescape(s string) string {
	return strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'",
		"&quot;", `"`,
	).Replace(s)
}

// Find is every element with a name.
func Find(elements []Element, name string) []Element {
	var out []Element

	for _, e := range elements {
		if e.Name == name {
			out = append(out, e)
		}
	}

	return out
}
