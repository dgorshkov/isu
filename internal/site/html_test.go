package site

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const wellFormed = `<!DOCTYPE html>
<html lang="en">
<head><title>T</title><meta charset="utf-8"></head>
<body>
<!-- a comment with a < in it -->
<main id="main"><p class="a">text <a href="x.html">link <span>inner</span></a></p>
<img src="og.png" alt="">
<br/>
</main>
</body>
</html>
`

func TestParseHTMLReadsElementsAttributesAndText(t *testing.T) {
	t.Parallel()

	elements, err := ParseHTML(wellFormed)
	require.NoError(t, err)

	require.Len(t, Find(elements, "a"), 1)
	require.Equal(t, "x.html", Find(elements, "a")[0].Attribute("href"))
	require.Contains(t, Find(elements, "a")[0].Text, "link",
		"an element's own text is what a screen reader reads")
	require.Contains(t, Find(elements, "a")[0].Text, "inner",
		"text inside a child belongs to the ancestor too")

	require.Empty(t, Find(elements, "p")[0].Attribute("id"), "an absent attribute is empty")
	require.Equal(t, "a", Find(elements, "p")[0].Attribute("class"))

	require.Len(t, Find(elements, "img"), 1, "a void element does not wait for a close tag")
	require.Len(t, Find(elements, "br"), 1, "a self-closing tag closes itself")

	require.Positive(t, Find(elements, "main")[0].Depth)
}

func TestParseHTMLRefusesWhatTwoBrowsersWouldReadDifferently(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		doc  string
		want string
	}{
		{"no doctype", "<html><body></body></html>", "does not open with <!DOCTYPE html>"},
		{
			"a tag that never closes",
			"<!DOCTYPE html>\n<html lang=\"en\"><body><p>x",
			"is never closed",
		},
		{
			"a close that matches nothing",
			"<!DOCTYPE html>\n</p>",
			"closes nothing",
		},
		{
			"a close in the wrong order",
			"<!DOCTYPE html>\n<div><p></div></p>",
			"</div> closes <p>",
		},
		{
			"an unquoted attribute value",
			"<!DOCTYPE html>\n<html lang=en></html>",
			"is not a quoted attribute value",
		},
		{
			"a `<` that never closes",
			"<!DOCTYPE html>\n<html lang=\"en\"><p",
			"never closes",
		},
		{
			"a tag with no name",
			"<!DOCTYPE html>\n< >",
			"a tag with no name",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseHTML(tt.doc)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

// An attribute value with no closing quote never reaches openTag through a
// document: tagEnd is looking for a `>` outside a quoted value and does not
// find one, so ParseHTML refuses the tag first. The branch is still there
// because openTag is not entitled to assume its caller, and it is asserted
// where it can be reached.
func TestOpenTagRefusesAnAttributeValueThatNeverCloses(t *testing.T) {
	t.Parallel()

	_, err := openTag(`a href="x`, 0)
	require.ErrorContains(t, err, "unclosed attribute value")

	_, err = ParseHTML(`<!DOCTYPE html>` + "\n" + `<html lang="en></html>`)
	require.ErrorContains(t, err, "never closes")
}

func TestParseHTMLReadsABarewordAttributeAndUnescapesValues(t *testing.T) {
	t.Parallel()

	elements, err := ParseHTML(
		`<!DOCTYPE html>` + "\n" + `<html lang="en"><input disabled>` +
			`<a href="a.html?x=1&amp;y=&#34;2&#34;">t</a></html>`)
	require.NoError(t, err)

	require.Contains(t, Find(elements, "input")[0].Attr, "disabled")
	require.Equal(t, `a.html?x=1&y="2"`, Find(elements, "a")[0].Attribute("href"))
}

// TestABarewordAttributeDoesNotHideTheNextOne is a defect this parser shipped
// with, and the way it was found is the point. Attributes were split on the
// first `=` anywhere in what was left of the tag, so `<script async src="…">`
// read as one attribute named `async src` and the element carried no src at
// all. gateLinks and gateThirdParty both ask an element for its src, so a
// valueless attribute written in front of one was enough to walk a script past
// every gate on this site.
func TestABarewordAttributeDoesNotHideTheNextOne(t *testing.T) {
	t.Parallel()

	elements, err := ParseHTML(
		`<!DOCTYPE html>` + "\n" + `<html lang="en">` +
			`<script async defer src="https://cdn.example/x.js"></script>` +
			`<input disabled readonly></html>`)
	require.NoError(t, err)

	script := Find(elements, "script")[0]
	require.Equal(t, "https://cdn.example/x.js", script.Attribute("src"))
	require.Contains(t, script.Attr, "async")
	require.Contains(t, script.Attr, "defer")

	require.Contains(t, Find(elements, "input")[0].Attr, "disabled")
	require.Contains(t, Find(elements, "input")[0].Attr, "readonly")
}

func TestAnUnfinishedCommentOrDoctypeEndsTheDocument(t *testing.T) {
	t.Parallel()

	for _, doc := range []string{
		"<!DOCTYPE html>\n<html lang=\"en\"></html><!-- never closed",
		"<!DOCTYPE html>\n<html lang=\"en\"></html><!unfinished",
	} {
		_, err := ParseHTML(doc)
		require.NoError(t, err)
	}
}

func TestTagEndIgnoresAGreaterThanInsideAQuotedValue(t *testing.T) {
	t.Parallel()

	elements, err := ParseHTML(
		`<!DOCTYPE html>` + "\n" + `<html lang="en"><meta content="a > b"></html>`)
	require.NoError(t, err)
	require.Equal(t, "a > b", Find(elements, "meta")[0].Attribute("content"))
}
