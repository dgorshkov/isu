package issue_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
)

// valid is the smallest document the parser accepts, used wherever a test is
// about one thing and needs the rest of the file to be uninteresting.
const valid = "---\nschema: 1\nid: ISU-7f3akq\n---\nBody.\n"

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		keys  []string
		want  map[string]string
		body  string
	}{
		{
			name:  "a valid document",
			input: valid,
			keys:  []string{"schema", "id"},
			want:  map[string]string{"schema": "1", "id": "ISU-7f3akq"},
			body:  "Body.\n",
		},
		{
			name:  "no body",
			input: "---\nid: ISU-7f3akq\n---\n",
			keys:  []string{"id"},
			want:  map[string]string{"id": "ISU-7f3akq"},
			body:  "",
		},
		{
			name:  "no keys",
			input: "---\n---\nJust a body.\n",
			keys:  []string{},
			want:  map[string]string{},
			body:  "Just a body.\n",
		},
		{
			name:  "CRLF line endings",
			input: "---\r\nid: ISU-7f3akq\r\ntype: bug\r\n---\r\nBody.\r\n",
			keys:  []string{"id", "type"},
			want:  map[string]string{"id": "ISU-7f3akq", "type": "bug"},
			body:  "Body.\r\n",
		},
		{
			name:  "unicode values",
			input: "---\ntitle: ünïcödé — 日本語 🐛\nowner: дмитрий\n---\nТело. 🦋\n",
			keys:  []string{"title", "owner"},
			want:  map[string]string{"title": "ünïcödé — 日本語 🐛", "owner": "дмитрий"},
			body:  "Тело. 🦋\n",
		},
		{
			name:  "a value containing colons",
			input: "---\nimported_at: 2026-08-24T09:12:00Z\n---\n",
			keys:  []string{"imported_at"},
			want:  map[string]string{"imported_at": "2026-08-24T09:12:00Z"},
			body:  "",
		},
		{
			name:  "an empty value",
			input: "---\nreason:\n---\n",
			keys:  []string{"reason"},
			want:  map[string]string{"reason": ""},
			body:  "",
		},
		{
			name:  "surrounding whitespace is not part of the value",
			input: "---\ntitle:    padded   \n---\n",
			keys:  []string{"title"},
			want:  map[string]string{"title": "padded"},
			body:  "",
		},
		{
			name:  "blank lines inside the block",
			input: "---\nid: ISU-7f3akq\n\ntype: bug\n---\n",
			keys:  []string{"id", "type"},
			want:  map[string]string{"id": "ISU-7f3akq", "type": "bug"},
			body:  "",
		},
		{
			name:  "unknown keys",
			input: "---\nid: ISU-7f3akq\njira_key: PROJ-1234\n---\n",
			keys:  []string{"id", "jira_key"},
			want:  map[string]string{"id": "ISU-7f3akq", "jira_key": "PROJ-1234"},
			body:  "",
		},
		{
			name:  "no newline at end of file",
			input: "---\nid: ISU-7f3akq\n---",
			keys:  []string{"id"},
			want:  map[string]string{"id": "ISU-7f3akq"},
			body:  "",
		},
		{
			name:  "a delimiter in the body closes nothing",
			input: "---\nid: ISU-7f3akq\n---\nBefore.\n---\nAfter.\n",
			keys:  []string{"id"},
			want:  map[string]string{"id": "ISU-7f3akq"},
			body:  "Before.\n---\nAfter.\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc, err := issue.Parse([]byte(tc.input))
			require.NoError(t, err)

			require.Equal(t, tc.keys, doc.Keys())
			require.Equal(t, tc.body, doc.Body)

			for key, want := range tc.want {
				got, ok := doc.Get(key)
				require.True(t, ok, "key %q is missing", key)
				require.Equal(t, want, got)
			}

			require.Equal(t, tc.input, doc.String(),
				"a document that was not modified must round-trip byte for byte")
		})
	}
}

func TestParseAOneMegabyteBody(t *testing.T) {
	t.Parallel()

	const line = "The quick brown fox jumps over the lazy dog.\n"

	body := strings.Repeat(line, 1<<20/len(line)+1)
	require.Greater(t, len(body), 1<<20)

	doc, err := issue.Parse([]byte("---\nid: ISU-7f3akq\n---\n" + body))
	require.NoError(t, err)
	require.Equal(t, body, doc.Body)
}

func TestParseRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		line  int
		msg   string
	}{
		{
			name:  "an empty file",
			input: "",
			line:  1,
			msg:   "empty",
		},
		{
			name:  "a file that does not open with a delimiter",
			input: "id: ISU-7f3akq\n---\n",
			line:  1,
			msg:   "must open with ---",
		},
		{
			name:  "a body with no frontmatter at all",
			input: "Just some markdown.\n",
			line:  1,
			msg:   "must open with ---",
		},
		{
			name:  "a missing close delimiter",
			input: "---\nid: ISU-7f3akq\ntype: bug\n",
			line:  3,
			msg:   "never closed",
		},
		{
			name:  "a duplicate key",
			input: "---\nid: ISU-7f3akq\ntype: bug\nid: ISU-40b1cc\n---\n",
			line:  4,
			msg:   `duplicate key "id", already set on line 2`,
		},
		{
			name:  "a line with no colon",
			input: "---\nid: ISU-7f3akq\nnonsense\n---\n",
			line:  3,
			msg:   "expected `key: value`",
		},
		{
			name:  "an empty key",
			input: "---\n: value\n---\n",
			line:  2,
			msg:   "the key is empty",
		},
		{
			name:  "a key with a space in it",
			input: "---\nissue id: ISU-7f3akq\n---\n",
			line:  2,
			msg:   "is not a key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc, err := issue.Parse([]byte(tc.input))
			require.Nil(t, doc)
			require.Error(t, err)

			var perr *issue.ParseError
			require.ErrorAs(t, err, &perr, "every parse failure is a *ParseError")
			require.Equal(t, tc.line, perr.Line)
			require.Contains(t, perr.Error(), tc.msg)
			require.Contains(t, perr.Error(), "line ")
		})
	}
}

func TestDocumentSetRewritesOneLineAndLeavesTheRest(t *testing.T) {
	t.Parallel()

	doc, err := issue.Parse([]byte("---\nid: ISU-7f3akq\nstate:   open\ntype: bug\n---\nBody.\n"))
	require.NoError(t, err)

	doc.Set("state", "resolved")

	require.Equal(t, "---\nid: ISU-7f3akq\nstate: resolved\ntype: bug\n---\nBody.\n", doc.String())
	require.Equal(t, []string{"id", "state", "type"}, doc.Keys())
}

func TestDocumentSetAppendsANewKeyAtTheEndOfTheBlock(t *testing.T) {
	t.Parallel()

	doc, err := issue.Parse([]byte("---\nid: ISU-7f3akq\n---\nBody.\n"))
	require.NoError(t, err)

	doc.Set("resolution", "wontfix")

	require.Equal(t, "---\nid: ISU-7f3akq\nresolution: wontfix\n---\nBody.\n", doc.String())
	require.Equal(t, []string{"id", "resolution"}, doc.Keys())
}

func TestDocumentSetKeepsCRLFTerminators(t *testing.T) {
	t.Parallel()

	doc, err := issue.Parse([]byte("---\r\nid: ISU-7f3akq\r\n---\r\n"))
	require.NoError(t, err)

	doc.Set("state", "open")

	require.Equal(t, "---\r\nid: ISU-7f3akq\r\nstate: open\r\n---\r\n", doc.String())
}

// A value carrying a line break would write a file that does not parse back,
// so Set flattens it rather than producing one.
func TestDocumentSetFlattensLineBreaks(t *testing.T) {
	t.Parallel()

	doc := issue.NewDocument()
	doc.Set("title", "first\r\nsecond\nthird\rfourth")

	require.Equal(t, "---\ntitle: first second third fourth\n---\n", doc.String())

	again, err := issue.Parse(doc.Bytes())
	require.NoError(t, err)

	got, ok := again.Get("title")
	require.True(t, ok)
	require.Equal(t, "first second third fourth", got)
}

func TestDocumentUnset(t *testing.T) {
	t.Parallel()

	doc, err := issue.Parse([]byte("---\nid: ISU-7f3akq\nstate: open\ntype: bug\n---\n"))
	require.NoError(t, err)

	doc.Unset("state")
	doc.Unset("state")
	doc.Unset("never-was-here")

	require.Equal(t, "---\nid: ISU-7f3akq\ntype: bug\n---\n", doc.String())
	require.Equal(t, []string{"id", "type"}, doc.Keys())
	require.False(t, doc.Has("state"))

	// The index has to survive the removal, or Set after Unset writes to the
	// wrong line.
	doc.Set("type", "story")
	require.Equal(t, "---\nid: ISU-7f3akq\ntype: story\n---\n", doc.String())
}

func TestDocumentHasSeparatesAbsentFromEmpty(t *testing.T) {
	t.Parallel()

	doc, err := issue.Parse([]byte("---\nreason:\n---\n"))
	require.NoError(t, err)

	require.True(t, doc.Has("reason"))
	require.False(t, doc.Has("resolution"))

	value, ok := doc.Get("reason")
	require.True(t, ok)
	require.Empty(t, value)

	value, ok = doc.Get("resolution")
	require.False(t, ok)
	require.Empty(t, value)
}

func TestNewDocument(t *testing.T) {
	t.Parallel()

	doc := issue.NewDocument()
	require.Equal(t, "---\n---\n", doc.String())
	require.Empty(t, doc.Keys())

	doc.Set("id", "ISU-7f3akq")
	doc.Body = "Body.\n"
	require.Equal(t, "---\nid: ISU-7f3akq\n---\nBody.\n", doc.String())
}

// The round trip is the property the whole format rests on: it is what keeps a
// pull request that touches one field to a one-line diff.
func TestRoundTripEveryFixture(t *testing.T) {
	t.Parallel()

	fixtures, err := filepath.Glob(filepath.Join("testdata", "issues", "*.md"))
	require.NoError(t, err)
	require.NotEmpty(t, fixtures)

	for _, path := range fixtures {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(path)
			require.NoError(t, err)

			doc, err := issue.Parse(data)
			require.NoError(t, err)
			require.Equal(t, string(data), doc.String(),
				"parse then serialise must be byte-identical")

			again, err := issue.Parse(doc.Bytes())
			require.NoError(t, err)
			require.Equal(t, doc, again,
				"parse, serialise and parse again must yield an identical document")
		})
	}
}

// Parse is fed whatever is in the repository, so the one thing it may never do
// is panic. The seed corpus runs on every `go test`.
func FuzzParse(f *testing.F) {
	f.Add(valid)
	f.Add("")
	f.Add("---")
	f.Add("---\n")
	f.Add("---\n---")
	f.Add("---\r\n\r\n---\r\n")
	f.Add("---\n:\n---\n")
	f.Add("---\na: 1\na: 2\n---\n")
	f.Add("--- \n\ta : b \n --- \nbody")
	f.Add("---\n\x00: \x00\n---\n")

	f.Fuzz(func(t *testing.T, input string) {
		doc, err := issue.Parse([]byte(input))
		if err != nil {
			return
		}

		if got := doc.String(); got != input {
			t.Fatalf("round trip changed the bytes:\n in: %q\nout: %q", input, got)
		}
	})
}
