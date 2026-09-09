package site

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// plan is a content plan with one of everything the grammar allows.
const plan = `# a brief

<!-- title: T -->
<!-- tagline: A tagline. -->
<!-- description: A description. -->
<!-- lede: A lede. -->
<!-- install: go install x -->

Prose above the page is the brief and is not the page.

## The page

### 1. First section
<!-- id: first -->
**Claim.** A claim that
wraps onto a second line.
**Proof.** ` + "`isu board`" + `, which is editorial and is not rendered.

A paragraph
that wraps.

Another paragraph.

` + "```console\n$ isu board\nmain\n```" + `

### Second section

No anchor, no claim, no sample.

## The design

### A heading after the page, which is not a section

` + "```console\n$ isu ready\n```" + `
`

func TestParsePlanReadsThePageAndNothingElse(t *testing.T) {
	t.Parallel()

	got, err := ParsePlan(plan)
	require.NoError(t, err)

	require.Len(t, got.Sections, 2, "only the sections under `## The page` are the page")

	first := got.Sections[0]
	require.Equal(t, "first", first.ID)
	require.Equal(t, "First section", first.Title, "the ordinal is stripped")
	require.Equal(t, "A claim that wraps onto a second line.", first.Claim)
	require.Equal(t, []string{"A paragraph that wraps.", "Another paragraph."}, first.Copy,
		"the proof is editorial and does not reach the page")
	require.Len(t, first.Samples, 1)
	require.Equal(t, "isu board", first.Samples[0].String())
	require.Equal(t, "main\n", first.Samples[0].Want)

	second := got.Sections[1]
	require.Equal(t, "Second section", second.Title, "a heading with no ordinal keeps its words")
	require.Empty(t, second.ID)
	require.Empty(t, second.Samples,
		"a console block after the page is not one of the page's samples")

	for key, want := range map[string]string{
		"title": "T", "tagline": "A tagline.", "description": "A description.",
		"lede": "A lede.", "install": "go install x",
	} {
		got, err := got.Get(key)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestParsePlanRefusesAPlanThatCannotBecomeAPage(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, doc, want string }{
		{"no page", "# a brief\n\n## Something else\n", `has no "## The page" heading`},
		{
			"a page with no sections",
			"## The page\n\nprose and nothing else\n\n```console\n$ isu board\n```\n",
			"declares no sections",
		},
		{
			"a page that shows nothing running",
			"## The page\n\n### 1. A section\n\nprose\n",
			"shows no output isu produced",
		},
		{
			"a plan this package will not read",
			"## The page\n\n### 1. A section\n\n$ isu board\n",
			"outside a ```console fence",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParsePlan(tt.doc)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestGetNamesTheKeyThePlanIsMissing(t *testing.T) {
	t.Parallel()

	p := Plan{Meta: map[string]string{"empty": ""}}

	_, err := p.Get("title")
	require.ErrorContains(t, err, "<!-- title: … -->")

	_, err = p.Get("empty")
	require.ErrorContains(t, err, "<!-- empty: … -->",
		"a key present and empty is a key that is not there")
}

func TestCommentsReadsOnlyWhatLooksLikeOne(t *testing.T) {
	t.Parallel()

	got := comments("<!-- a: 1 -->\n<!-- no colon -->\nnot a comment\n<!-- unterminated\n" +
		"<!--   spaced   :   out   -->\n")

	require.Equal(t, map[string]string{"a": "1", "spaced": "out"}, got)
}

func TestCutSectionStopsAtTheNextHeadingOfItsOwnLevel(t *testing.T) {
	t.Parallel()

	body, ok := cutSection("## A\n\none\n\n## B\n\ntwo\n", "## A")
	require.True(t, ok)
	require.Equal(t, "\none\n", body)

	_, ok = cutSection("## A\n", "## Z")
	require.False(t, ok)
}

func TestOrdinalOnlyStripsSomethingThatIsOne(t *testing.T) {
	t.Parallel()

	require.Equal(t, "A section", ordinal("1. A section"))
	require.Equal(t, "A section", ordinal("A section"))
	require.Equal(t, "Mr. Smith regrets", ordinal("Mr. Smith regrets"),
		"a full stop is not an ordinal unless what precedes it is a number")
}

func TestABlockThePlanCannotParseIsNotASample(t *testing.T) {
	t.Parallel()

	// consoleSamples is reached with fences that parseBlock refuses only when
	// ParsePlan has already accepted the document, which cannot happen through
	// ParsePlan. It is asserted here, where it can be.
	fences, _ := scanFences(
		"```console\n```\n```sh\nnot a console block\n```\n```console\n$ isu board\nmain\n```\n")
	require.Len(t, consoleSamples(fences), 1,
		"an empty block contributes no sample, and neither does another kind of fence")
}

// The plan describes the stylesheet, twice in a table, and prose about a file is
// prose that stops being true. It already had: the type table went on saying
// 1.25rem after --text-l became a clamp.
func TestThePlansDesignTablesAreHeldToTheStylesheet(t *testing.T) {
	t.Parallel()

	css := stylesheet(t)

	require.ErrorContains(t,
		TokensAgree("| `--nonesuch` | x | y |\n", css),
		"the stylesheet declares no such token")

	require.ErrorContains(t,
		TokensAgree("| `--paper` | `#ffffff` | `#000000` | the page |\n", css),
		"does not carry its light value")

	require.ErrorContains(t,
		TokensAgree("| `--paper` | `#fbf9f5` | `#000000` | the page |\n", css),
		"does not carry its dark value")

	require.ErrorContains(t, TokensAgree("", "body { color: red }"),
		"declares no :root block")

	require.ErrorContains(t, TokensAgree("", ":root {\n\t--paper: #fbf9f5;\n}\n"),
		"no dark colour scheme")
}
