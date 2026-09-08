package site

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkdownRendersTheSubsetAndNothingElse(t *testing.T) {
	t.Parallel()

	doc := `# A title

The lede, with **strong**, *emphasis*, ` + "`code`" + ` and a [link](checks.html).

## A section

- one
- two

1. first
2. second

> a quotation
> that wraps

| head | other |
|---|---|
| a | b |

` + "```go\nfmt.Println(\"<hi>\")\n```" + `

---

### Deeper

#### Deeper still
`

	got, err := Markdown(doc)
	require.NoError(t, err)

	require.Equal(t, "A title", got.Title)
	require.Equal(t, "The lede, with strong, emphasis, code and a link.", got.Lede,
		"the description is the lede with its markup taken off")
	require.Equal(t, []Heading{{Text: "A section", ID: "a-section"}}, got.Headings)

	for _, want := range []string{
		"<strong>strong</strong>", "<em>emphasis</em>", "<code>code</code>",
		`<a href="checks.html">link</a>`,
		`<h2 id="a-section">A section</h2>`,
		"<ul>\n<li>one</li>\n<li>two</li>\n</ul>",
		"<ol>\n<li>first</li>\n<li>second</li>\n</ol>",
		"<blockquote><p>a quotation that wraps</p></blockquote>",
		"<thead><tr>\n<th>head</th>\n<th>other</th>\n</tr></thead>",
		"<td>a</td>", "<hr>",
		`<div class="scroller sketch" tabindex="0" role="region" aria-label="terminal output">` +
			`<pre><code>fmt.Println(&#34;&lt;hi&gt;&#34;)</code></pre></div>`,
		`<h3 id="deeper">Deeper</h3>`, `<h4 id="deeper-still">Deeper still</h4>`,
	} {
		require.Contains(t, got.HTML, want)
	}
}

// The dark ground on this site means "these are the bytes the binary wrote".
// A block nothing ran must not be able to borrow it.
func TestOnlyABlockThatRanLooksLikeOne(t *testing.T) {
	t.Parallel()

	got, err := Markdown("# t\n\n" +
		"```console\n$ isu board\nmain\n```\n\n" +
		"```sh\nisu claim <id>\n```\n")
	require.NoError(t, err)

	require.Contains(t, got.HTML, `<div class="scroller ran" tabindex="0"`,
		"a console fence is executed by the build, so it keeps the terminal ground")
	require.Contains(t, got.HTML, `<div class="scroller sketch" tabindex="0"`,
		"every other fence is illustrative and must be visibly not a transcript")
}

func TestMarkdownRefusesWhatItCannotRender(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		doc  string
		want string
	}{
		{"no title", "just a paragraph\n", "no `# ` heading"},
		{"two titles", "# one\n\n# two\n", "a page has one title"},
		{"a heading six deep", "# one\n\n##### too deep\n", "not a heading this renderer knows"},
		{"a hash that is not a heading", "# one\n\n#nospace\n", "not a heading this renderer knows"},
		{"a table with no rule", "# one\n\n| head |\n| a |\n", "needs a |---| row"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Markdown(tt.doc)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// Nothing in a source document may become markup. This is the whole reason
// there is no sanitiser: there is nothing for one to do.
func TestNothingInASourceDocumentBecomesMarkup(t *testing.T) {
	t.Parallel()

	got, err := Markdown("# t\n\n<script>alert(1)</script> & \"quoted\"\n")
	require.NoError(t, err)
	require.NotContains(t, got.HTML, "<script>")
	require.Contains(t, got.HTML, "&lt;script&gt;")
	require.Contains(t, got.HTML, "&amp;")
}

func TestInlineLeavesUnfinishedMarkupAsText(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ in, want string }{
		{"a ` backtick", "a ` backtick"},
		{"a ** strong", "a ** strong"},
		{"a * star", "a * star"},
		{"a [ bracket", "a [ bracket"},
		{"a [text] with no href", "a [text] with no href"},
		{"a [text](unclosed", "a [text](unclosed"},
		{"a — dash and a é", "a — dash and a é"},
	} {
		require.Equal(t, tt.want, Inline(tt.in))
	}
}

func TestPlainSurvivesMarkupItCannotStrip(t *testing.T) {
	t.Parallel()

	require.Equal(t, "a [text] left alone", plain("a [text] left alone"))
	require.Equal(t, "a link", plain("a [link](x.html)"))
}

func TestSlugIsSomethingAUrlCanCarry(t *testing.T) {
	t.Parallel()

	require.Equal(t, "the-data-model", Slug("The data model"))
	require.Equal(t, "isu-check-and-friends", Slug("  `isu check` — and friends!  "))
	require.Empty(t, Slug("—"))
}

func TestOrderedPrefixIsOnlyADigitRunFollowedByADotAndASpace(t *testing.T) {
	t.Parallel()

	require.Equal(t, "12. ", orderedPrefix("12. twelfth"))
	require.Empty(t, orderedPrefix("no digits"))
	require.Empty(t, orderedPrefix("12.no space"))
}

func TestAListEndsWhereItsMarkerDoes(t *testing.T) {
	t.Parallel()

	got, err := Markdown("# t\n\n1. first\nnot a list any more\n")
	require.NoError(t, err)
	require.Contains(t, got.HTML, "<ol>\n<li>first</li>\n</ol>")
	require.Contains(t, got.HTML, "<p>not a list any more</p>")
}

func TestARuleRowIsARuleRowAndNothingElse(t *testing.T) {
	t.Parallel()

	require.True(t, isRule("|---|:---:|"))
	require.False(t, isRule("not a row"))
	require.False(t, isRule("| a | b |"))
}
