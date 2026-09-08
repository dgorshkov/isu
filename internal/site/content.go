package site

import (
	"fmt"
	"strings"
)

// The landing page is built from web/CONTENT.md and from nothing else.
//
// M8-S1 writes the content plan and M8-S2 says "build the page from
// CONTENT.md". Read strictly, that is a rule about where the copy lives: a
// template that carried a sentence of its own would be a second place the
// site's words are written, and the two would drift the first time somebody
// edited the one they happened to open. So the template carries structure and
// the plan carries every word, and this file is the seam between them.
//
// The grammar is the smallest thing that can say what a section is:
//
//	### 2. Status is derived, never stored
//	<!-- id: derived -->
//	**Claim.** A tracker that stores status is a tracker somebody keeps in sync.
//	**Proof.** `isu board`, against the sample repository.
//
//	Copy, one paragraph at a time.
//
//	```console
//	$ isu board
//	...
//	```
//
// `**Proof.**` is editorial: it names the artifact for the reviewer and is not
// rendered. Everything else is.

// Section is one section of the landing page.
type Section struct {
	// ID is the anchor, from `<!-- id: … -->`.
	ID string
	// Title is the heading, with its ordinal stripped.
	Title string
	// Claim is the one thing the section asserts.
	Claim string
	// Copy is the body, one entry per paragraph, as inline markdown.
	Copy []string
	// Sample is the terminal block that proves the claim, or nil when the
	// section proves itself with words.
	Sample *Command
}

// Plan is web/CONTENT.md, read.
type Plan struct {
	// Meta is the `<!-- key: value -->` block above the sections: the page
	// title, its tagline, its description.
	Meta map[string]string
	// Sections are the page, in order.
	Sections []Section
}

// Get is one metadata value, and an error naming the key when the plan does
// not carry it — a page with no description is a page the gates refuse, and
// finding that out here names the file to edit.
func (p Plan) Get(key string) (string, error) {
	value, ok := p.Meta[key]
	if !ok || value == "" {
		return "", fmt.Errorf("web/CONTENT.md declares no <!-- %s: … -->", key)
	}

	return value, nil
}

// pageHeading opens the part of CONTENT.md that is the page. Everything before
// it is the brief, and everything after the next heading of the same level is
// the docs tree and the design record.
const pageHeading = "## The page"

// ParsePlan reads the landing page out of the content plan.
func ParsePlan(doc string) (Plan, error) {
	plan := Plan{Meta: map[string]string{}}

	body, ok := cutSection(doc, pageHeading)
	if !ok {
		return Plan{}, fmt.Errorf("web/CONTENT.md has no %q heading", pageHeading)
	}

	blocks, err := Blocks(doc)
	if err != nil {
		return Plan{}, err
	}

	fences, _ := scanFences(body)
	samples := consoleSamples(fences)

	for key, value := range comments(doc) {
		plan.Meta[key] = value
	}

	plan.Sections = sections(body, samples)

	if len(plan.Sections) == 0 {
		return Plan{}, fmt.Errorf("web/CONTENT.md declares no sections under %q", pageHeading)
	}

	if len(blocks) == 0 {
		return Plan{}, fmt.Errorf("web/CONTENT.md shows no output isu produced")
	}

	return plan, nil
}

// cutSection is the body of one `##` section: everything after its heading and
// before the next heading at the same level.
func cutSection(doc, heading string) (string, bool) {
	_, after, found := strings.Cut(doc, heading+"\n")
	if !found {
		return "", false
	}

	var kept []string

	for _, line := range strings.Split(after, "\n") {
		if strings.HasPrefix(line, "## ") {
			break
		}

		kept = append(kept, line)
	}

	return strings.Join(kept, "\n"), true
}

// comments reads every `<!-- key: value -->` line in a document.
func comments(doc string) map[string]string {
	out := map[string]string{}

	for _, line := range strings.Split(doc, "\n") {
		inner, ok := strings.CutPrefix(strings.TrimSpace(line), "<!--")
		if !ok {
			continue
		}

		inner, ok = strings.CutSuffix(strings.TrimSpace(inner), "-->")
		if !ok {
			continue
		}

		key, value, ok := strings.Cut(inner, ":")
		if !ok {
			continue
		}

		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return out
}

// consoleSamples is the first command of each console block in a body, in
// order, paired with what the plan claims it printed. The claim has already
// been held to the product by Verify; here it is the sample the page shows.
func consoleSamples(fences []fenced) []Command {
	var out []Command

	for _, f := range fences {
		if !isConsole(f.info) {
			continue
		}

		block, err := parseBlock(f)
		if err != nil || len(block.Commands) == 0 {
			continue
		}

		out = append(out, block.Commands[0])
	}

	return out
}

// sections splits the page body into its `###` sections.
func sections(body string, samples []Command) []Section {
	var (
		out     []Section
		current *Section
		para    []string
		used    int
		// label is which of a section's labelled paragraphs is open. A claim
		// and a proof are both paragraphs rather than lines: one sentence
		// wraps, and a parser that took only the first line would put the rest
		// of it into the copy as a fragment.
		label string
	)

	flush := func() {
		defer func() { para, label = nil, "" }()

		if current == nil || len(para) == 0 {
			return
		}

		switch label {
		case "claim":
			current.Claim = strings.Join(para, " ")
		case "proof":
		default:
			current.Copy = append(current.Copy, strings.Join(para, " "))
		}
	}

	finish := func() {
		flush()

		if current != nil {
			out = append(out, *current)
			current = nil
		}
	}

	inFence := false

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "```"):
			flush()

			inFence = !inFence
			if !inFence && current != nil && used < len(samples) {
				sample := samples[used]
				used++
				current.Sample = &sample
			}
		case inFence:
		case strings.HasPrefix(line, "### "):
			finish()

			current = &Section{Title: ordinal(strings.TrimPrefix(line, "### "))}
		case current == nil:
		case strings.HasPrefix(trimmed, "<!--"):
			if id, ok := comments(line)["id"]; ok {
				current.ID = id
			}
		case strings.HasPrefix(trimmed, "**Proof.**"):
			flush()

			label = "proof"
			para = []string{strings.TrimSpace(strings.TrimPrefix(trimmed, "**Proof.**"))}
		case strings.HasPrefix(trimmed, "**Claim.**"):
			flush()

			label = "claim"
			para = []string{strings.TrimSpace(strings.TrimPrefix(trimmed, "**Claim.**"))}
		case trimmed == "":
			flush()
		default:
			para = append(para, trimmed)
		}
	}

	finish()

	return out
}

// ordinal strips the `2. ` a heading carries so that the plan reads as an
// ordered document and the page does not read as a numbered list.
func ordinal(heading string) string {
	number, rest, found := strings.Cut(strings.TrimSpace(heading), ". ")
	if !found {
		return strings.TrimSpace(heading)
	}

	for _, r := range number {
		if r < '0' || r > '9' {
			return strings.TrimSpace(heading)
		}
	}

	return strings.TrimSpace(rest)
}
