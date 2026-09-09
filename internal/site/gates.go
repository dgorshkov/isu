package site

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// The gates M8-S3 asks for, run over the built site rather than over a running
// browser: an internal link checker, HTML validity, a per-page weight budget,
// an accessibility pass, a width the page survives at, and reduced motion.
//
// They run inside Build, so `make site` refuses to write a site that fails one
// and the test that regenerates the site fails for the same reason. A gate that
// only the test enforces is a gate somebody bypasses the first time they run
// the generator by hand.
//
// What they can and cannot see is worth stating plainly, because a gate that
// oversells itself is worse than none. There is no browser here, so "no
// horizontal scroll at 360 px" is enforced as the two things that cause it —
// a fixed width wider than the viewport, and wide content outside a scrolling
// box — rather than measured. The contrast pass computes WCAG's own ratio from
// the declared tokens, which is exact for the pairs the page actually puts
// together and says nothing about a pair nobody wrote down.

// PageBudget is the weight one page may cost: its own bytes plus every
// same-origin resource it loads. PLAN.md M8-S3 sets it at 300 KB.
const PageBudget = 300 * 1024

// Narrowest is the viewport the page must survive, in CSS pixels.
const Narrowest = 360

// Gates runs every gate over a built site and reports the first failure.
func Gates(files map[string][]byte) error {
	css := string(files["site.css"])

	for _, gate := range []func(map[string][]byte, string) error{
		gateHTML, gateLinks, gateWeight, gateAccessibility, gateMeta,
		gateThirdParty, gateContrast, gateWidth, gateMotion, gateTokens, gateProse,
		gateFocus, gateDecoration,
	} {
		if err := gate(files, css); err != nil {
			return err
		}
	}

	return nil
}

// pages is every HTML file in a built site, in a stable order.
func pages(files map[string][]byte) []string {
	var out []string

	for path := range files {
		if strings.HasSuffix(path, ".html") {
			out = append(out, path)
		}
	}

	sort.Strings(out)

	return out
}

// gateHTML is the validity pass: every page parses, and nothing in it is
// ambiguous enough that two browsers would disagree.
func gateHTML(files map[string][]byte, _ string) error {
	for _, page := range pages(files) {
		elements, err := ParseHTML(string(files[page]))
		if err != nil {
			return fmt.Errorf("%s: %w", page, err)
		}

		if err := singletons(elements); err != nil {
			return fmt.Errorf("%s: %w", page, err)
		}

		seen := map[string]bool{}

		for _, e := range elements {
			id := e.Attribute("id")
			if id == "" {
				continue
			}

			if seen[id] {
				return fmt.Errorf(`%s: two elements carry id="%s"`, page, id)
			}

			seen[id] = true
		}
	}

	return nil
}

// singletons holds the structural elements a document has exactly one of.
func singletons(elements []Element) error {
	for _, name := range []string{"html", "head", "body", "title", "main"} {
		if n := len(Find(elements, name)); n != 1 {
			return fmt.Errorf("a document has one <%s>, and this one has %d", name, n)
		}
	}

	if lang := Find(elements, "html")[0].Attribute("lang"); lang == "" {
		return fmt.Errorf("<html> declares no lang")
	}

	return nil
}

// resources is every same-origin file a page loads, as site-relative paths.
//
// An anchor is not a resource: a link to another site is a link somebody
// clicks, not a request the page makes. What this collects is what the browser
// fetches to render the page, which is what the weight budget and the
// third-party rule are both about.
func resources(page string, elements []Element) []string {
	var out []string

	for _, e := range elements {
		var ref string

		switch e.Name {
		case "link":
			if rel := e.Attribute("rel"); rel == "stylesheet" || rel == "icon" {
				ref = e.Attribute("href")
			}
		case "script", "img", "source", "video", "audio", "iframe":
			ref = e.Attribute("src")
		}

		if ref == "" || external(ref) {
			continue
		}

		out = append(out, path.Join(path.Dir(page), ref))
	}

	return out
}

func external(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") ||
		strings.HasPrefix(ref, "//") || strings.HasPrefix(ref, "data:")
}

// gateLinks is the internal link checker: every relative href and src resolves
// to a file this build produced, and every fragment resolves to an id on the
// page it names.
func gateLinks(files map[string][]byte, _ string) error {
	anchors := map[string]map[string]bool{}

	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))
		ids := map[string]bool{}

		for _, e := range elements {
			if id := e.Attribute("id"); id != "" {
				ids[id] = true
			}
		}

		anchors[page] = ids
	}

	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		for _, ref := range references(elements) {
			if err := resolve(files, anchors, page, ref); err != nil {
				return fmt.Errorf("%s: %w", page, err)
			}
		}
	}

	return nil
}

// references is every href and src on a page, as the document writes them:
// resources and anchors alike, and relative to the page rather than to the
// site. Resolving them is resolve's job and is done exactly once.
func references(elements []Element) []string {
	var out []string

	for _, e := range elements {
		for _, attr := range []string{"href", "src"} {
			if ref := e.Attribute(attr); ref != "" && !external(ref) {
				out = append(out, ref)
			}
		}
	}

	return out
}

// resolve checks one reference against the built site.
func resolve(files map[string][]byte, anchors map[string]map[string]bool, page, ref string) error {
	target, fragment, _ := strings.Cut(ref, "#")

	if target == "" {
		target = page
	} else {
		target = path.Join(path.Dir(page), target)
	}

	if _, ok := files[target]; !ok {
		return fmt.Errorf("%s points at %s, which this build does not produce", ref, target)
	}

	if fragment != "" && !anchors[target][fragment] {
		return fmt.Errorf("%s points at an id %s does not carry", ref, target)
	}

	return nil
}

// gateWeight holds every page to PageBudget: its own bytes, plus everything it
// makes the browser fetch.
func gateWeight(files map[string][]byte, _ string) error {
	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		weight := len(files[page])
		counted := map[string]bool{}

		for _, ref := range resources(page, elements) {
			if counted[ref] {
				continue
			}

			counted[ref] = true
			weight += len(files[ref])
		}

		if weight > PageBudget {
			return fmt.Errorf("%s weighs %d bytes against a budget of %d",
				page, weight, PageBudget)
		}
	}

	return nil
}

// headingLevel is 1 for h1, 0 for anything else.
func headingLevel(name string) int {
	if len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6' {
		return int(name[1] - '0')
	}

	return 0
}

// gateAccessibility is the pass M8-S3 asks for, at the depth a document can be
// read at: one h1, no skipped heading levels, alt text on every image,
// discernible text in every link, a labelled navigation and a skip link.
func gateAccessibility(files map[string][]byte, _ string) error {
	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		if n := len(Find(elements, "h1")); n != 1 {
			return fmt.Errorf("%s has %d <h1>, and a page has one", page, n)
		}

		previous := 0

		for _, e := range elements {
			level := headingLevel(e.Name)
			if level > 0 {
				if previous > 0 && level > previous+1 {
					return fmt.Errorf("%s goes from <h%d> to <h%d>, skipping a level",
						page, previous, level)
				}

				previous = level
			}

			if err := labelled(e); err != nil {
				return fmt.Errorf("%s: %w", page, err)
			}
		}

		if err := landmarks(elements); err != nil {
			return fmt.Errorf("%s: %w", page, err)
		}
	}

	return nil
}

// labelled is the per-element half: an image says what it shows, and a link
// says where it goes.
func labelled(e Element) error {
	switch e.Name {
	case "img":
		if _, ok := e.Attr["alt"]; !ok {
			return fmt.Errorf("<img src=%q> carries no alt", e.Attribute("src"))
		}
	case "a":
		if strings.TrimSpace(e.Text) == "" && e.Attribute("aria-label") == "" {
			return fmt.Errorf("<a href=%q> has no text a screen reader can read",
				e.Attribute("href"))
		}
	}

	return nil
}

// landmarks is the per-page half: a navigation somebody can tell from another
// navigation, and a way past it.
func landmarks(elements []Element) error {
	for _, nav := range Find(elements, "nav") {
		if nav.Attribute("aria-label") == "" {
			return fmt.Errorf("a <nav> with no aria-label is one of two nobody can tell apart")
		}
	}

	for _, a := range Find(elements, "a") {
		if strings.HasPrefix(a.Attribute("href"), "#") {
			return nil
		}
	}

	return fmt.Errorf("the page has no skip link past its masthead")
}

// scrollingSelectors are the selectors gateFocus knows how to evaluate. It is
// a fixed list on purpose: a stylesheet that makes something else scroll fails
// the build until this list and the markup that satisfies it both grow.
var scrollingSelectors = map[string]bool{".install": true, ".proof pre": true, ".scroller": true}

// gateFocus is the accessibility rule this build shipped without and should
// not have.
//
// A box with overflow-x is a box a keyboard user has to be able to reach and
// scroll — WCAG 2.1.1, and axe's scrollable-region-focusable. Every terminal
// card on this site is one, which made it the most-repeated element here and
// the one the accessibility pass walked straight past: gateAccessibility was
// written to check what a document *says* (alt text, link text, headings) and
// nothing about what the stylesheet then does to it.
//
// So this reads the stylesheet rather than a list of element names. Whatever
// the CSS gives an overflow-x of its own has to carry a tabindex in the markup,
// and a scrolling selector this gate cannot evaluate is a failure rather than a
// silence — which is the half of the original bug that mattered.
func gateFocus(files map[string][]byte, css string) error {
	scrolls, err := scrollingRules(css)
	if err != nil {
		return err
	}

	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		for i, e := range elements {
			if !scrolls[matching(elements[:i], e)] || e.Attribute("tabindex") != "" {
				continue
			}

			return fmt.Errorf(
				"%s: a <%s> the stylesheet scrolls carries no tabindex, so a keyboard "+
					"cannot reach it", page, e.Name)
		}
	}

	return nil
}

// matching is the selector in scrollingSelectors that an element answers to, or
// the empty string.
func matching(before []Element, e Element) string {
	switch {
	case slices.Contains(strings.Fields(e.Attribute("class")), "scroller"):
		return ".scroller"
	case slices.Contains(strings.Fields(e.Attribute("class")), "install"):
		return ".install"
	case e.Name == "pre" && ancestorHasClass(before, e, "proof"):
		return ".proof pre"
	}

	return ""
}

// ancestorHasClass walks out of an element through the elements that enclose it.
func ancestorHasClass(before []Element, e Element, class string) bool {
	want := e.Depth - 1

	for i := len(before) - 1; i >= 0 && want >= 0; i-- {
		if before[i].Depth != want {
			continue
		}

		want--

		if slices.Contains(strings.Fields(before[i].Attribute("class")), class) {
			return true
		}
	}

	return false
}

// scrollingRules is every selector the stylesheet gives an overflow-x of its
// own, checked against the ones gateFocus can evaluate.
func scrollingRules(css string) (map[string]bool, error) {
	out := map[string]bool{}

	for _, block := range strings.Split(stripComments(css), "}") {
		selector, body, found := strings.Cut(block, "{")
		if !found || !strings.Contains(body, "overflow-x: auto") {
			continue
		}

		name := strings.TrimSpace(selector)
		if i := strings.LastIndex(name, "{"); i >= 0 {
			name = strings.TrimSpace(name[i+1:])
		}

		if !scrollingSelectors[name] {
			return nil, fmt.Errorf(
				"the stylesheet scrolls %q and gateFocus does not know how to check it", name)
		}

		out[name] = true
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("nothing in the stylesheet scrolls sideways")
	}

	return out, nil
}

// stripComments removes /* … */ so a selector is never read out of prose.
func stripComments(css string) string {
	var b strings.Builder

	rest := css

	for {
		before, after, found := strings.Cut(rest, "/*")
		b.WriteString(before)

		if !found {
			return b.String()
		}

		_, rest, found = strings.Cut(after, "*/")
		if !found {
			return b.String()
		}
	}
}

// saysLinked is prose promising a link.
var saysLinked = regexp.MustCompile(`\blinked\b|\blinks to\b`)

// gateProse is the narrowest possible answer to a real failure: the landing
// page said "the out-of-scope page is linked from here rather than buried" and
// carried no anchor at all.
//
// It is worth being exact about what this does and does not do. The build gates
// every *sample* on this site against the product, and it gates nothing a
// sentence claims — prose is not checkable in general and this does not pretend
// otherwise. What it checks is one bug class that a site whose whole argument is
// "what this page says is mechanically true" cannot afford twice: a section
// that tells the reader something is linked, and then is not.
func gateProse(files map[string][]byte, _ string) error {
	elements, err := ParseHTML(string(files["index.html"]))
	if err != nil {
		return err
	}

	for i, e := range elements {
		if e.Name != "section" {
			continue
		}

		if !saysLinked.MatchString(e.Text) {
			continue
		}

		if !linksSomewhere(elements[i+1:], e.Depth) {
			return fmt.Errorf(
				"index.html: section %q says something is linked and carries no link",
				e.Attribute("id"))
		}
	}

	return nil
}

// linksSomewhere reports whether an anchor appears before the element that
// opened at depth closes.
func linksSomewhere(after []Element, depth int) bool {
	for _, e := range after {
		if e.Depth <= depth {
			return false
		}

		if e.Name == "a" {
			return true
		}
	}

	return false
}

// gateDecoration keeps decoration out of the text layer.
//
// The page's one call to action was written as
// `<span aria-hidden="true">$ </span>{{.Install}}<span class="caret" aria-hidden="true">_</span>`,
// and aria-hidden hides a string from a screen reader and not from a clipboard.
// Nothing on this site sets user-select, so a reader who selected the only
// command the landing page asks them to run and pasted it got
// `$ go install github.com/dgorshkov/isu/cmd/isu@latest_`, which does not run.
// The proof card three hundred lines away had been doing the same prompt as a
// pseudo-element since the day it was written.
//
// So the rule is one sentence: anything hidden from assistive technology is
// decoration, and decoration belongs in the stylesheet. Text inside an
// aria-hidden element is a string the clipboard gets and the screen reader does
// not, which is the wrong way round in both directions.
func gateDecoration(files map[string][]byte, _ string) error {
	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		for _, e := range elements {
			text := strings.TrimSpace(e.Text)
			if e.Attribute("aria-hidden") != "true" || text == "" {
				continue
			}

			return fmt.Errorf(
				"%s: a <%s> hidden from a screen reader carries the text %q, which a "+
					"reader still copies — put decoration in the stylesheet",
				page, e.Name, text)
		}
	}

	return nil
}

// gateMeta is what a page needs before anybody has opened it: a title, a
// description, a canonical URL and a card.
func gateMeta(files map[string][]byte, _ string) error {
	want := []string{"og:title", "og:description", "og:image", "og:url"}

	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		if strings.TrimSpace(Find(elements, "title")[0].Text) == "" {
			return fmt.Errorf("%s has an empty <title>", page)
		}

		named := map[string]string{}

		for _, meta := range Find(elements, "meta") {
			if key := meta.Attribute("name") + meta.Attribute("property"); key != "" {
				named[key] = meta.Attribute("content")
			}
		}

		for _, key := range append(want, "description", "twitter:card", "viewport") {
			if named[key] == "" {
				return fmt.Errorf("%s declares no %s", page, key)
			}
		}

		if err := canonical(page, elements); err != nil {
			return err
		}
	}

	return nil
}

func canonical(page string, elements []Element) error {
	for _, l := range Find(elements, "link") {
		if l.Attribute("rel") == "canonical" && strings.HasPrefix(l.Attribute("href"), SiteURL) {
			return nil
		}
	}

	return fmt.Errorf("%s declares no canonical URL under %s", page, SiteURL)
}

// gateThirdParty is M8-S2's "zero third-party network requests": nothing the
// browser fetches to render a page comes from anywhere but this site.
func gateThirdParty(files map[string][]byte, css string) error {
	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		for _, e := range elements {
			for _, attr := range []string{"src", "href"} {
				ref := e.Attribute(attr)
				if !external(ref) || !fetched(e, attr) {
					continue
				}

				return fmt.Errorf("%s loads %s from another origin", page, ref)
			}
		}
	}

	for _, match := range regexp.MustCompile(`url\(([^)]*)\)`).FindAllStringSubmatch(css, -1) {
		if external(strings.Trim(match[1], `'"`)) {
			return fmt.Errorf("the stylesheet fetches %s from another origin", match[1])
		}
	}

	if strings.Contains(css, "@import") {
		return fmt.Errorf("the stylesheet imports another stylesheet")
	}

	return nil
}

// fetched reports whether an attribute names something the browser loads
// rather than something a reader clicks.
func fetched(e Element, attr string) bool {
	if attr == "src" {
		return true
	}

	rel := e.Attribute("rel")

	return e.Name == "link" && (rel == "stylesheet" || rel == "icon" || rel == "preload")
}

// contrastPairs are the foreground and background the page actually puts
// together, and the ratio WCAG asks of each. Body text is AA at 4.5; nothing on
// this site is large enough to claim the 3:1 allowance, so nothing does.
var contrastPairs = []struct {
	fg, bg string
	min    float64
}{
	{"ink", "paper", 4.5},
	{"slate", "paper", 4.5},
	{"signal", "paper", 4.5},
	{"terminal-ink", "terminal", 4.5},
	{"glow", "terminal", 4.5},
}

// gateContrast computes WCAG's ratio for every pair above, in both schemes.
//
// This is the gate that earns its keep: --signal on --terminal is 3.2:1 and
// looks fine, which is exactly the kind of thing that ships without a number
// attached to it.
func gateContrast(_ map[string][]byte, css string) error {
	light, err := Palette(css)
	if err != nil {
		return err
	}

	dark, err := DarkPalette(css)
	if err != nil {
		return err
	}

	schemes := []struct {
		name    string
		palette map[string]string
	}{{"light", light}, {"dark", dark}}

	for _, scheme := range schemes {
		for _, pair := range contrastPairs {
			ratio, err := Contrast(scheme.palette, pair.fg, pair.bg)
			if err != nil {
				return err
			}

			if ratio < pair.min {
				return fmt.Errorf(
					"%s: --%s on --%s is %.2f:1, and the page needs %.1f:1",
					scheme.name, pair.fg, pair.bg, ratio, pair.min)
			}
		}
	}

	return nil
}

// Contrast is the WCAG 2.1 ratio between two named colours in a palette.
func Contrast(palette map[string]string, fg, bg string) (float64, error) {
	a, err := pick(palette, fg)
	if err != nil {
		return 0, err
	}

	b, err := pick(palette, bg)
	if err != nil {
		return 0, err
	}

	first, second := luminance(a.R, a.G, a.B), luminance(b.R, b.G, b.B)
	if first < second {
		first, second = second, first
	}

	return (first + 0.05) / (second + 0.05), nil
}

// luminance is WCAG's relative luminance.
func luminance(r, g, b uint8) float64 {
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}

func channel(v uint8) float64 {
	f := float64(v) / 255
	if f <= 0.03928 {
		return f / 12.92
	}

	return math.Pow((f+0.055)/1.055, 2.4)
}

// pixelWidth finds a declared width in px.
var pixelWidth = regexp.MustCompile(`(?m)^\s*(min-width|width)\s*:\s*(\d+)px`)

// gateWidth is "responsive down to 360 px", enforced as the two things that
// cause a horizontal scrollbar rather than measured in a browser this build
// does not have: a fixed width wider than the viewport, and wide content that
// is not in a box of its own that scrolls.
func gateWidth(files map[string][]byte, css string) error {
	for _, match := range pixelWidth.FindAllStringSubmatch(css, -1) {
		width, _ := strconv.Atoi(match[2])
		if width > Narrowest {
			return fmt.Errorf("the stylesheet sets %s: %dpx, which is wider than %dpx",
				match[1], width, Narrowest)
		}
	}

	if !strings.Contains(css, "overflow-x: auto") {
		return fmt.Errorf("nothing in the stylesheet scrolls sideways, so wide output cannot")
	}

	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		for i, e := range elements {
			if e.Name != "pre" && e.Name != "table" {
				continue
			}

			if !scrollable(elements[:i], e) {
				return fmt.Errorf("%s: a <%s> is not inside a box that scrolls sideways",
					page, e.Name)
			}
		}
	}

	return nil
}

// scrollable reports whether an element sits inside one of the two containers
// the stylesheet gives an overflow-x of its own.
func scrollable(before []Element, e Element) bool {
	want := e.Depth - 1

	for i := len(before) - 1; i >= 0 && want >= 0; i-- {
		if before[i].Depth != want {
			continue
		}

		want--

		class := before[i].Attribute("class")
		if slices.Contains(strings.Fields(class), "scroller") || class == "proof" {
			return true
		}
	}

	return false
}

// gateMotion is PLAN.md M8-S3's `prefers-reduced-motion` honoured: a stylesheet
// that animates must also say what it does for somebody who asked it not to.
func gateMotion(_ map[string][]byte, css string) error {
	animates := strings.Contains(css, "animation:") || strings.Contains(css, "transition:")
	if !animates {
		return nil
	}

	if !strings.Contains(css, "prefers-reduced-motion: reduce") {
		return fmt.Errorf("the stylesheet animates and never asks about reduced motion")
	}

	return nil
}

// hexLiteral is a colour written out rather than referred to.
var hexLiteral = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)

// pixelFont is a type size written in device pixels.
var pixelFont = regexp.MustCompile(`font-size\s*:[^;]*\bpx\b|font-size\s*:\s*\d+px`)

// gateTokens is M8-S2's "no template hardcodes a hex value or a pixel font
// size": every colour and every type size is declared once, in a :root block,
// and referred to by name everywhere else.
func gateTokens(files map[string][]byte, css string) error {
	outside := withoutRootBlocks(css)

	if match := hexLiteral.FindString(outside); match != "" {
		return fmt.Errorf("the stylesheet writes %s outside its token blocks", match)
	}

	if match := pixelFont.FindString(css); match != "" {
		return fmt.Errorf("the stylesheet sets a type size in pixels: %s", match)
	}

	// A page can only carry a colour of its own in a style attribute or a
	// <style> element, so those are what is looked at: searching the whole
	// document would flag every anchor whose slug happens to be six hex
	// characters, and `#decade` is a heading somebody will write one day.
	for _, page := range pages(files) {
		elements, _ := ParseHTML(string(files[page]))

		if n := len(Find(elements, "style")); n > 0 {
			return fmt.Errorf("%s carries a <style> element; the stylesheet is the one place", page)
		}

		for _, e := range elements {
			if match := hexLiteral.FindString(e.Attribute("style")); match != "" {
				return fmt.Errorf("%s writes the colour %s into a style attribute", page, match)
			}
		}
	}

	return nil
}

// withoutRootBlocks is the stylesheet with the bodies of its `:root` blocks
// removed, which is everything that is not allowed to name a colour.
func withoutRootBlocks(css string) string {
	var b strings.Builder

	rest := css

	for {
		before, after, found := strings.Cut(rest, ":root {")
		if !found {
			b.WriteString(rest)

			return b.String()
		}

		b.WriteString(before)

		_, rest, found = strings.Cut(after, "\n}")
		if !found {
			return b.String()
		}
	}
}

// DarkPalette is the token block a reader in a dark colour scheme gets.
func DarkPalette(css string) (map[string]string, error) {
	_, after, found := strings.Cut(css, "prefers-color-scheme: dark")
	if !found {
		return nil, fmt.Errorf("the stylesheet has no dark colour scheme")
	}

	dark, err := Palette(after)
	if err != nil {
		return nil, err
	}

	light, err := Palette(css)
	if err != nil {
		return nil, err
	}

	// A dark block redeclares what changes and inherits the rest.
	for name, value := range light {
		if _, ok := dark[name]; !ok {
			dark[name] = value
		}
	}

	return dark, nil
}
