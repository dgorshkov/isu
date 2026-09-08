package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The failure paths of a build are reached the way PLAN.md's definition of done
// says they are reached in this project: with a directory that is not what the
// code expects, and with a path something else is already sitting on. Nothing
// here depends on running unprivileged.

func TestBuildSaysWhichSourceItCannotRead(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, missing, want string }{
		{"the templates", filepath.Join("web", "templates"), "reading the site's templates"},
		{"the stylesheet", filepath.Join("web", "assets", "site.css"), "site.css"},
		{"the content plan", filepath.Join("web", "CONTENT.md"), "CONTENT.md"},
		{"a document", filepath.Join("docs", Documents[0].Name+".md"), Documents[0].Name},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Build(t.Context(), copyWithout(t, tt.missing), t.TempDir())
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestBuildRefusesAContentPlanThatDoesNotMatchTheProduct(t *testing.T) {
	t.Parallel()

	root := copyWithout(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(root, "web", "CONTENT.md"),
		[]byte("## The page\n\n### 1. A section\n<!-- id: a -->\n**Claim.** c\n\nprose\n\n"+
			"```console\n$ isu board\nnot what isu says\n```\n"), 0o600))

	_, err := Build(t.Context(), root, t.TempDir())
	require.ErrorContains(t, err, "web/CONTENT.md")
	require.ErrorContains(t, err, "and the document claims")
}

func TestBuildRefusesAPlanWithNoMetadata(t *testing.T) {
	t.Parallel()

	root := copyWithout(t, "")

	body, err := os.ReadFile(filepath.Join(root, "web", "CONTENT.md"))
	require.NoError(t, err)

	stripped := removeLinePrefixed(string(body), "<!-- title:")
	require.NotEqual(t, string(body), stripped)

	require.NoError(t, os.WriteFile(filepath.Join(root, "web", "CONTENT.md"),
		[]byte(stripped), 0o600))

	_, err = Build(t.Context(), root, t.TempDir())
	require.ErrorContains(t, err, "declares no <!-- title:")
}

func TestBuildRefusesADocumentTheRendererCannotRead(t *testing.T) {
	t.Parallel()

	root := copyWithout(t, "")
	name := Documents[1].Name

	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", name+".md"),
		[]byte("no heading at all\n"), 0o600))

	_, err := Build(t.Context(), root, t.TempDir())
	require.ErrorContains(t, err, "docs/"+name+".md")
	require.ErrorContains(t, err, "no `# ` heading")
}

func TestBuildRefusesASiteThatFailsAGate(t *testing.T) {
	t.Parallel()

	root := copyWithout(t, "")

	css, err := os.ReadFile(filepath.Join(root, "web", "assets", "site.css"))
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(root, "web", "assets", "site.css"),
		append(css, []byte("\n.x { color: #ff0000; }\n")...), 0o600))

	_, err = Build(t.Context(), root, t.TempDir())
	require.ErrorContains(t, err, "outside its token blocks")
}

func TestFixturesRefuseADirectoryTheyCannotBuildIn(t *testing.T) {
	t.Parallel()

	for name, build := range map[string]func(t *testing.T, dir string) error{
		"the sample repository": func(t *testing.T, dir string) error {
			t.Helper()

			_, err := Fixture(t.Context(), dir)

			return err
		},
		"a repository the documentation writes to": func(t *testing.T, dir string) error {
			t.Helper()

			_, err := Writable(t.Context(), dir)

			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// A file where the repository's folder belongs: MkdirAll cannot
			// make a directory there.
			dir := t.TempDir()
			occupied := filepath.Join(dir, "taken")
			require.NoError(t, os.WriteFile(occupied, []byte("in the way"), 0o600))

			require.Error(t, build(t, occupied))
		})
	}
}

func TestAFixtureStopsAtItsFirstFailure(t *testing.T) {
	t.Parallel()

	// git will not initialise a repository inside a path that is a file, so
	// every step after the first is skipped and the first error is the one
	// reported.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), nil, 0o600))

	s := newScratch(t.Context(), filepath.Join(dir, "file", "repo"))
	build(s)

	require.Error(t, s.err)
}

func TestWriteRefusesAPathSomethingElseIsSittingOn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs"), []byte("a file"), 0o600))
	require.ErrorContains(t,
		Write(dir, map[string][]byte{"docs/index.html": []byte("x")}), "docs")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "index.html"), 0o750))
	require.ErrorContains(t, Write(dir, map[string][]byte{"index.html": []byte("x")}), "index.html")
}

func TestWritePutsTheSiteOnDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, Write(dir, map[string][]byte{
		"index.html":      []byte("page"),
		"docs/index.html": []byte("docs"),
	}))

	for path, want := range map[string]string{"index.html": "page", "docs/index.html": "docs"} {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		require.NoError(t, err)
		require.Equal(t, want, string(got))
	}
}

// copyWithout is this repository's site sources, copied into a temporary tree,
// with one path left out. It is how a build is given a source it cannot read
// without touching the repository the tests run in.
func copyWithout(t *testing.T, skip string) string {
	t.Helper()

	dir := t.TempDir()

	for _, from := range []string{
		filepath.Join("web", "CONTENT.md"),
		filepath.Join("web", "assets", "site.css"),
		filepath.Join("web", "templates", "pages.html.tmpl"),
	} {
		copyFile(t, dir, from, skip)
	}

	for _, d := range Documents {
		copyFile(t, dir, filepath.Join("docs", d.Name+".md"), skip)
	}

	return dir
}

func copyFile(t *testing.T, dir, from, skip string) {
	t.Helper()

	if skip != "" && (from == skip || filepath.Dir(from) == skip) {
		return
	}

	body, err := os.ReadFile(filepath.Join(root, from))
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(from)), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, from), body, 0o600))
}

// removeLinePrefixed drops every line a document opens with a given prefix.
func removeLinePrefixed(doc, prefix string) string {
	var kept []string

	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, prefix) {
			continue
		}

		kept = append(kept, line)
	}

	return strings.Join(kept, "\n")
}

func TestBuildRefusesAStylesheetItCannotReadTokensFrom(t *testing.T) {
	t.Parallel()

	root := copyWithout(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(root, "web", "assets", "site.css"),
		[]byte("body { color: red }\n"), 0o600))

	_, err := Build(t.Context(), root, t.TempDir())
	require.ErrorContains(t, err, "declares no :root block")
}

// A stylesheet with tokens but not the ones the images need gets as far as the
// icons, which is what puts the assembler's two refusals on the record: the
// first failure is kept, and everything after it is skipped.
func TestBuildRefusesAStylesheetTheImagesCannotUse(t *testing.T) {
	t.Parallel()

	root := copyWithout(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(root, "web", "assets", "site.css"),
		[]byte(":root {\n\t--paper: #ffffff;\n}\n"), 0o600))

	_, err := Build(t.Context(), root, t.TempDir())
	require.ErrorContains(t, err, "declares no --terminal")
}

func TestBuildRefusesAWorkingDirectoryItCannotBuildAFixtureIn(t *testing.T) {
	t.Parallel()

	work := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(work, nil, 0o600))

	_, err := Build(t.Context(), copyWithout(t, ""), work)
	require.Error(t, err)
}

// A plan every command in which runs, and which is still not a page.
func TestBuildRefusesAPlanThatIsNotAPage(t *testing.T) {
	t.Parallel()

	root := copyWithout(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(root, "web", "CONTENT.md"),
		[]byte("# a brief\n\n```console\n$ isu board\n```\n"), 0o600))

	_, err := Build(t.Context(), root, t.TempDir())
	require.ErrorContains(t, err, `has no "## The page" heading`)
}

func TestASummaryIsThePagesOwnFirstSentence(t *testing.T) {
	t.Parallel()

	got, err := summary("An issue is a folder. Everything else follows from that.")
	require.NoError(t, err)
	require.Equal(t, "An issue is a folder.", got,
		"the index says what the page says about itself, and says it once")

	got, err = summary("A lede that is one sentence and carries no full stop after it")
	require.NoError(t, err)
	require.Equal(t, "A lede that is one sentence and carries no full stop after it", got)

	// The case this gate was written for: a reference token wearing a sentence's
	// full stop. docs/field-notes.md really did open "M4-S8."
	_, err = summary("M4-S8. Everything before this ran against fixtures.")
	require.ErrorContains(t, err, `opens with "M4-S8."`)
	require.ErrorContains(t, err, "too short to say what the page is for")
}

func TestBuildRefusesADocumentThatCannotSayWhatItIsFor(t *testing.T) {
	t.Parallel()

	root := copyWithout(t, "")
	name := Documents[2].Name

	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", name+".md"),
		[]byte("# A title\n\nM4-S8. Then a paragraph that is long enough on its own.\n"), 0o600))

	_, err := Build(t.Context(), root, t.TempDir())
	require.ErrorContains(t, err, "docs/"+name+".md")
	require.ErrorContains(t, err, "too short to say what the page is for")
}
