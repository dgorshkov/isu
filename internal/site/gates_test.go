package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The gates are asserted the way a gate has to be: by giving each one the
// violation it exists to catch and requiring it to say so. A gate nobody has
// ever seen fail is a gate whose rule is a comment — the same argument
// scripts/coverage.sh makes about the harness.

// stylesheet is the real one. The colour and motion gates are about a specific
// stylesheet, and testing them against an invented one would assert that this
// package can read CSS rather than that this site clears the bar.
func stylesheet(t *testing.T) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(root, "web", "assets", "site.css"))
	require.NoError(t, err)

	return string(body)
}

// smallSite is the smallest site that clears every gate: one page that says
// everything a page has to say, and the project's own stylesheet.
func smallSite(t *testing.T) map[string][]byte {
	t.Helper()

	return map[string][]byte{
		"site.css":    []byte(stylesheet(t)),
		"favicon.svg": []byte("<svg></svg>"),
		"index.html":  []byte(smallPage("")),
	}
}

// smallPage is that page, with extra markup spliced into its body.
func smallPage(extra string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>T</title>
<meta name="description" content="d">
<link rel="canonical" href="` + SiteURL + `/index.html">
<link rel="icon" href="favicon.svg" type="image/svg+xml">
<link rel="stylesheet" href="site.css">
<meta property="og:title" content="T">
<meta property="og:description" content="d">
<meta property="og:image" content="` + SiteURL + `/og.png">
<meta property="og:url" content="` + SiteURL + `/index.html">
<meta name="twitter:card" content="summary_large_image">
</head>
<body>
<a class="skip" href="#main">Skip</a>
<nav aria-label="Sections"><a href="index.html">Home</a></nav>
<main id="main">
<h1>T</h1>
` + extra + `</main>
</body>
</html>
`
}

func TestTheGatesPassASiteThatMeetsThem(t *testing.T) {
	t.Parallel()

	require.NoError(t, Gates(smallSite(t)))
}

// Each row breaks exactly one rule.
func TestEachGateCatchesTheThingItIsFor(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{"not well formed", "<p>never closed\n", "</main> closes <p>"},
		{"two elements with one id", `<p id="a"></p><p id="a"></p>`, "two elements carry"},
		{"a second h1", "<h1>again</h1>\n", "has 2 <h1>"},
		{"a second main", `<div><main id="two">t</main></div>`, "has one <main>, and this one has 2"},
		{"a skipped heading level", "<h4>deep</h4>\n", "skipping a level"},
		{"an image with no alt", `<img src="favicon.svg">`, "carries no alt"},
		{"a link with no text", `<a href="index.html"></a>`, "no text a screen reader"},
		{"a link to nothing", `<p><a href="gone.html">g</a></p>`, "this build does not produce"},
		{"a fragment nothing carries", `<p><a href="#nope">n</a></p>`, "an id index.html does not carry"},
		{
			"a stylesheet from somewhere else",
			`<p><a href="index.html">a</a></p>`,
			"another origin",
		},
		{
			"a script from somewhere else, behind a bareword attribute",
			`<p><script async src="https://cdn.example/x.js"></script></p>`,
			"another origin",
		},
		{"a pre that cannot scroll", "<pre>wide</pre>\n", "not inside a box that scrolls"},
		{"a colour written into the page", `<p style="color: #ff0000">x</p>`, "into a style attribute"},
		{"a style element", "<style>p{}</style>\n", "carries a <style> element"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := smallSite(t)
			files["index.html"] = []byte(smallPage(tt.body))

			if tt.name == "a stylesheet from somewhere else" {
				files["index.html"] = []byte(strings.Replace(string(files["index.html"]),
					`<link rel="stylesheet" href="site.css">`,
					`<link rel="stylesheet" href="https://cdn.example/site.css">`, 1))
			}

			require.ErrorContains(t, Gates(files), tt.want)
		})
	}
}

func TestTheStructuralGatesCatchAMissingLandmark(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		replace [2]string
		want    string
	}{
		{"no lang", [2]string{`<html lang="en">`, `<html>`}, "declares no lang"},
		{
			"an unlabelled nav",
			[2]string{`<nav aria-label="Sections">`, `<nav>`},
			"nobody can tell apart",
		},
		{
			"no skip link",
			[2]string{`<a class="skip" href="#main">Skip</a>`, `<p>no skip</p>`},
			"no skip link past its masthead",
		},
		{"an empty title", [2]string{`<title>T</title>`, `<title></title>`}, "empty <title>"},
		{
			"no description",
			[2]string{`<meta name="description" content="d">`, ``},
			"declares no description",
		},
		{
			"no card",
			[2]string{`<meta name="twitter:card" content="summary_large_image">`, ``},
			"declares no twitter:card",
		},
		{
			"a canonical somewhere else",
			[2]string{
				SiteURL + `/index.html">` + "\n" + `<link rel="icon"`,
				`https://example.com/">` + "\n" + `<link rel="icon"`,
			},
			"declares no canonical URL",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := smallSite(t)
			page := strings.Replace(smallPage(""), tt.replace[0], tt.replace[1], 1)
			require.NotEqual(t, smallPage(""), page, "the fixture no longer says %q", tt.replace[0])

			files["index.html"] = []byte(page)
			require.ErrorContains(t, Gates(files), tt.want)
		})
	}
}

func TestAWideMainCloseTagIsStillMatched(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["index.html"] = []byte(smallPage(
		`<div class="scroller" tabindex="0"><pre>wide</pre></div>` + "\n"))

	require.NoError(t, Gates(files), "a pre inside a scroller is allowed to be wide")
}

func TestTheWeightBudgetIsAPageAndEverythingItLoads(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["site.css"] = []byte(stylesheet(t) + "\n/*" + strings.Repeat("x", PageBudget) + "*/\n")

	require.ErrorContains(t, Gates(files), "against a budget of")
}

func TestTheStylesheetGatesReadTheStylesheet(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		replace [2]string
		want    string
	}{
		{
			"a colour outside the token blocks",
			[2]string{"color: var(--ink);", "color: #123456;"},
			"outside its token blocks",
		},
		{
			"a type size in pixels",
			[2]string{"font-size: var(--text-m);", "font-size: 16px;"},
			"a type size in pixels",
		},
		{
			"a fixed width wider than the narrowest viewport",
			[2]string{"\tmargin: 0;\n\tbackground: var(--paper);", "\twidth: 900px;"},
			"which is wider than 360px",
		},
		{
			"animation with no way out of it",
			[2]string{"prefers-reduced-motion: reduce", "prefers-contrast: more"},
			"never asks about reduced motion",
		},
		{
			"no dark scheme at all",
			[2]string{"prefers-color-scheme: dark", "prefers-contrast: more"},
			"no dark colour scheme",
		},
		{
			"a pair nobody can read",
			[2]string{
				"--glow: #f0ac7a;\n\n\t/* The type scale",
				"--glow: #1c1a18;\n\n\t/* The type scale",
			},
			"--glow on --terminal is",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := smallSite(t)
			css := strings.Replace(stylesheet(t), tt.replace[0], tt.replace[1], 1)
			require.NotEqual(t, stylesheet(t), css,
				"the stylesheet no longer says %q", tt.replace[0])

			files["site.css"] = []byte(css)
			require.ErrorContains(t, Gates(files), tt.want)
		})
	}
}

func TestAStylesheetWhereNothingScrollsSidewaysIsRefused(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["site.css"] = []byte(
		strings.ReplaceAll(stylesheet(t), "overflow-x: auto", "overflow-x: visible"))

	require.ErrorContains(t, Gates(files), "so wide output cannot")
}

func TestAStylesheetThatFetchesFromAnotherOriginIsRefused(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ suffix, want string }{
		{"\n.x { background: url(https://cdn.example/a.png); }\n", "fetches"},
		{"\n@import url(other.css);\n", "imports another stylesheet"},
	} {
		files := smallSite(t)
		files["site.css"] = []byte(stylesheet(t) + tt.suffix)

		require.ErrorContains(t, Gates(files), tt.want)
	}
}

func TestAStylesheetWithNoTokensIsRefused(t *testing.T) {
	t.Parallel()

	_, err := Palette("body { color: red }")
	require.ErrorContains(t, err, "declares no :root block")

	_, err = Palette(":root {\n\tcolor: red;\n}\n")
	require.ErrorContains(t, err, "declares no custom properties")
}

func TestTheContrastGateNeedsAStylesheetItCanRead(t *testing.T) {
	t.Parallel()

	t.Run("no tokens at all", func(t *testing.T) {
		t.Parallel()

		files := smallSite(t)
		files["site.css"] = []byte("body { }\n")

		require.ErrorContains(t, Gates(files), "declares no :root block")
	})

	t.Run("tokens, but not the ones the page puts together", func(t *testing.T) {
		t.Parallel()

		files := smallSite(t)
		files["site.css"] = []byte(":root {\n\t--paper: #ffffff;\n}\n" +
			"@media (prefers-color-scheme: dark) {\n\t:root {\n\t\t--paper: #000000;\n\t}\n}\n")

		require.ErrorContains(t, Gates(files), "declares no --ink")
	})
}

// A stylesheet that animates nothing has nothing to turn off, and the motion
// gate says so by passing rather than by demanding a media query for a
// transition that is not there.
func TestTheMotionGatePassesAStylesheetThatDoesNotAnimate(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["site.css"] = []byte(strings.ReplaceAll(
		strings.ReplaceAll(stylesheet(t), "animation:", "anim-x:"),
		"prefers-reduced-motion: reduce", "prefers-contrast: more"))

	require.NoError(t, Gates(files))
}

func TestDarkTokensInheritWhatTheyDoNotRedeclare(t *testing.T) {
	t.Parallel()

	dark, err := DarkPalette(stylesheet(t))
	require.NoError(t, err)

	require.Equal(t, "#f2eee7", dark["ink"], "the dark block redeclares --ink")
	require.Equal(t, "0.9rem", dark["text-s"], "and inherits the type scale")

	_, err = DarkPalette("body{}")
	require.ErrorContains(t, err, "no dark colour scheme")

	_, err = DarkPalette("@media (prefers-color-scheme: dark) { body { } }")
	require.ErrorContains(t, err, "declares no :root block",
		"a dark block with no tokens in it is a stylesheet nobody can read")

	_, err = DarkPalette(":root {\n\tcolor: red;\n}\n" +
		"@media (prefers-color-scheme: dark) {\n\t:root {\n\t\t--paper: #000000;\n\t}\n}\n")
	require.ErrorContains(t, err, "declares no custom properties",
		"the light block is what the dark one inherits from, so it has to be readable too")
}

func TestContrastIsWcagsOwnRatio(t *testing.T) {
	t.Parallel()

	palette := map[string]string{"black": "#000000", "white": "#ffffff", "half": "#767676"}

	ratio, err := Contrast(palette, "black", "white")
	require.NoError(t, err)
	require.InDelta(t, 21.0, ratio, 0.01, "black on white is the maximum")

	ratio, err = Contrast(palette, "white", "black")
	require.NoError(t, err)
	require.InDelta(t, 21.0, ratio, 0.01, "the ratio does not care which is which")

	ratio, err = Contrast(palette, "half", "white")
	require.NoError(t, err)
	require.InDelta(t, 4.54, ratio, 0.01, "#767676 is the classic AA boundary on white")

	_, err = Contrast(palette, "nothing", "white")
	require.ErrorContains(t, err, "declares no --nothing")

	_, err = Contrast(palette, "white", "nothing")
	require.ErrorContains(t, err, "declares no --nothing")
}

func TestWithoutRootBlocksSurvivesAnUnfinishedOne(t *testing.T) {
	t.Parallel()

	require.Equal(t, "a\n\nb\n", withoutRootBlocks("a\n:root {\n--x: 1;\n}\nb\n"))
	require.Equal(t, "a\n", withoutRootBlocks("a\n:root {\n--x: 1;\n"))
}

func TestHeadingLevelIsOnlyAHeading(t *testing.T) {
	t.Parallel()

	require.Equal(t, 3, headingLevel("h3"))
	require.Equal(t, 0, headingLevel("header"))
	require.Equal(t, 0, headingLevel("hr"))
	require.Equal(t, 0, headingLevel("p"))
}

func TestExternalIsEveryWayToNameAnotherOrigin(t *testing.T) {
	t.Parallel()

	for _, ref := range []string{
		"https://example.com/a", "http://example.com/a", "//example.com/a", "data:text/css,a",
	} {
		require.True(t, external(ref), ref)
	}

	require.False(t, external("docs/index.html"))
}

func TestASrcFromAnotherOriginIsAlwaysARequest(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["index.html"] = []byte(smallPage(
		`<p><img src="https://example.com/a.png" alt=""></p>` + "\n"))

	require.ErrorContains(t, Gates(files), "another origin")
}

func TestAPreloadIsFetchedAndAnAnchorIsNot(t *testing.T) {
	t.Parallel()

	require.True(t, fetched(Element{Name: "img", Attr: map[string]string{}}, "src"))
	require.True(t, fetched(
		Element{Name: "link", Attr: map[string]string{"rel": "preload"}}, "href"))
	require.False(t, fetched(Element{Name: "a", Attr: map[string]string{}}, "href"))
}

// The landing page said "the out-of-scope page is linked from here rather than
// buried" and carried no anchor at all. The samples on this site are gated
// against the product and a sentence is gated against nothing, so this covers
// the one bug class a site arguing "what this page says is true" cannot afford
// twice.
func TestASectionThatSaysSomethingIsLinkedHasToLinkIt(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["index.html"] = []byte(smallPage(
		`<section id="limits"><h2>Limits</h2><p>The page is linked from here.</p></section>`))

	require.ErrorContains(t, Gates(files),
		`section "limits" says something is linked and carries no link`)

	files["index.html"] = []byte(smallPage(
		`<section id="limits"><h2>Limits</h2>` +
			`<p>The page is <a href="index.html">linked</a> from here.</p></section>`))
	require.NoError(t, Gates(files), "a section that links what it says it links is fine")

	files["index.html"] = []byte(smallPage(
		`<section id="quiet"><h2>Quiet</h2><p>This section promises nothing.</p></section>`))
	require.NoError(t, Gates(files), "prose that claims no link needs none")
}

// The accessibility pass shipped without this and should not have. A box the
// stylesheet scrolls is a box a keyboard has to be able to reach, and every
// terminal card on this site is one — which made it the most repeated element
// here and the one nothing checked.
func TestABoxThatScrollsHasToBeReachable(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["index.html"] = []byte(smallPage(
		`<div class="scroller"><pre>wide output nothing can focus</pre></div>` + "\n"))

	require.ErrorContains(t, Gates(files), "carries no tabindex")
	require.ErrorContains(t, Gates(files), "a keyboard cannot reach it")

	files["index.html"] = []byte(smallPage(
		`<div class="scroller" tabindex="0"><pre>reachable</pre></div>` + "\n"))
	require.NoError(t, Gates(files))
}

// The half of the original bug that mattered: the gate did not know what it was
// not checking. A stylesheet that scrolls something new fails until this gate
// and the markup satisfying it both grow.
func TestAScrollingSelectorTheFocusGateCannotEvaluateIsAFailure(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["site.css"] = []byte(stylesheet(t) + "\n.brand-new { overflow-x: auto; }\n")

	require.ErrorContains(t, Gates(files),
		`the stylesheet scrolls ".brand-new" and gateFocus does not know how to check it`)
}

func TestScrollingRulesReadsTheStylesheetAndNotItsProse(t *testing.T) {
	t.Parallel()

	got, err := scrollingRules(stylesheet(t))
	require.NoError(t, err)
	require.Equal(t, map[string]bool{".install": true, ".proof pre": true, ".scroller": true}, got)

	_, err = scrollingRules("body { color: red }")
	require.ErrorContains(t, err, "nothing in the stylesheet scrolls sideways")

	// A selector named inside a comment is prose, not a rule.
	_, err = scrollingRules("/* .ghost { overflow-x: auto; } */\n.scroller { overflow-x: auto; }")
	require.NoError(t, err)
}

func TestStripCommentsSurvivesAnUnclosedOne(t *testing.T) {
	t.Parallel()

	require.Equal(t, "a b", stripComments("a /* gone */b"))
	require.Equal(t, "a ", stripComments("a /* never closed"))
	require.Equal(t, "plain", stripComments("plain"))
}

// The defect this gate exists for: the page's one call to action was a command
// with two aria-hidden spans wrapped round it, so a reader who selected it and
// pasted it got a prompt and a caret in their shell.
func TestNothingHiddenFromAScreenReaderCarriesTextAReaderCopies(t *testing.T) {
	t.Parallel()

	files := smallSite(t)
	files["index.html"] = []byte(smallPage(
		`<p><span aria-hidden="true">$ </span>go install example.com/x@latest</p>` + "\n"))

	require.ErrorContains(t, Gates(files),
		`hidden from a screen reader carries the text "$"`)

	files["index.html"] = []byte(smallPage(
		`<p><span aria-hidden="true"></span>go install example.com/x@latest</p>` + "\n"))

	require.NoError(t, Gates(files),
		"an empty hidden element is decoration the stylesheet fills in, which is the point")
}
