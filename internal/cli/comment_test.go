package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

func commentable(t *testing.T) *gittest.Repo {
	t.Helper()

	return configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Something to talk about")).
		Commit("report ISU-openly")
}

func TestCommentAppendsAFileAndShowRendersIt(t *testing.T) {
	t.Parallel()

	r := commentable(t)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "comment", "ISU-openly",
		"-m", "It happens on Safari too.").ok(t))

	require.Equal(t, "ISU-openly", written.ID)
	require.Len(t, written.Paths, 1)

	name := filepath.Base(written.Paths[0])
	require.Equal(t, today()+"-isu-tester-01.md", name,
		"<date>-<author>-<nn>.md, with the author slugged into a legal file name")

	require.Equal(t, "It happens on Safari too.\n",
		r.ReadFile("issues/ISU-openly/comments/"+name))

	// Commenting is one command and the result renders.
	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))
	require.Len(t, payload.Comments, 1)
	require.Equal(t, "It happens on Safari too.\n", payload.Comments[0].Body)

	text := isu(t, r.Dir(), "show", "ISU-openly").ok(t)
	require.Contains(t, text.stdout, "It happens on Safari too.")
}

func TestTwoCommentsBySamePersonOnSameDayDoNotOverwriteEachOther(t *testing.T) {
	t.Parallel()

	r := commentable(t)

	first := decode[Write](t, isu(t, r.Dir(), "--json", "comment", "ISU-openly",
		"-m", "The first thing").ok(t))
	second := decode[Write](t, isu(t, r.Dir(), "--json", "comment", "ISU-openly",
		"-m", "The second thing").ok(t))

	require.Equal(t, today()+"-isu-tester-01.md", filepath.Base(first.Paths[0]))
	require.Equal(t, today()+"-isu-tester-02.md", filepath.Base(second.Paths[0]))

	// The two-digit sequence is not decoration: without it the second silently
	// overwrites the first.
	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))
	require.Len(t, payload.Comments, 2)
	require.Equal(t, "The first thing\n", payload.Comments[0].Body)
	require.Equal(t, "The second thing\n", payload.Comments[1].Body)
}

func TestTheSequenceComesFromWhatIsAlreadyThere(t *testing.T) {
	t.Parallel()

	// Two people commenting on two branches keep no shared count, so the folder
	// is the only thing that knows — including when the numbering starts from
	// something other than one.
	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Talked about already"),
			gittest.Comment(today()+"-isu-tester-01", "an earlier one\n"),
			gittest.Comment(today()+"-isu-tester-07", "and a much later one\n"),
			gittest.Comment(today()+"-someone-else-01", "somebody else entirely\n"),
			gittest.Comment("not-a-comment-at-all.txt", "ignored\n")).
		Commit("report ISU-openly")

	written := decode[Write](t, isu(t, r.Dir(), "--json", "comment", "ISU-openly",
		"-m", "the next one").ok(t))

	require.Equal(t, today()+"-isu-tester-08.md", filepath.Base(written.Paths[0]))
}

func TestACommentDoesNotTouchTheIssue(t *testing.T) {
	t.Parallel()

	r := commentable(t)

	isu(t, r.Dir(), "comment", "ISU-openly", "-m", "a note").ok(t)

	_, err := r.Try("diff", "--exit-code", "--", "issues/ISU-openly/README.md")
	require.NoError(t, err, "commenting rewrites no part of the issue itself")

	_, err = r.Try("diff", "--cached", "--exit-code", "--", "issues/ISU-openly/README.md")
	require.NoError(t, err)
}

func TestACommentStartingALineWithThreeDashesIsJustMarkdown(t *testing.T) {
	t.Parallel()

	r := commentable(t)

	body := "Before.\n---\nAfter, under a horizontal rule.\n"

	written := decode[Write](t, isu(t, r.Dir(), "--json", "comment", "ISU-openly",
		"-m", body).ok(t))

	// Nothing parses a comment as a frontmatter document, so a rule at the
	// start of a line is a rule.
	require.Equal(t, body, r.ReadFile(written.Paths[0]))

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))
	require.Equal(t, body, payload.Comments[0].Body)
	require.Equal(t, "Something to talk about", payload.Issue.Title,
		"and the issue beside it still reads")
}

func TestACommentOnAnIssueThatIsNotThereIsRefused(t *testing.T) {
	t.Parallel()

	r := commentable(t)

	got := isu(t, r.Dir(), "comment", "ISU-nobody", "-m", "hello?")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "no issue ISU-nobody")
}

func TestAnEmptyCommentIsNotAComment(t *testing.T) {
	t.Parallel()

	r := commentable(t)

	got := isu(t, r.Dir(), "comment", "ISU-openly", "-m", "   \n\n  ")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "nothing to say")
}

func TestCommentOpensTheEditorWhenThereIsNoMessage(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the editor stand-in is a shell script; CI is linux and macos")
	}

	r := commentable(t)
	editor := writeEditor(t, "written in the editor\n")

	written := decode[Write](t, isuIn(t, r.Dir(), map[string]string{"EDITOR": editor},
		"--json", "comment", "ISU-openly").ok(t))

	require.Equal(t, "written in the editor\n", r.ReadFile(written.Paths[0]))
}

func TestTheEditorIsChosenInOrderAndFallsBackToVi(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"ISU_EDITOR wins", map[string]string{
			"ISU_EDITOR": "first", "VISUAL": "second", "EDITOR": "third",
		}, "first"},
		{"then VISUAL", map[string]string{"VISUAL": "second", "EDITOR": "third"}, "second"},
		{"then EDITOR", map[string]string{"EDITOR": "third"}, "third"},
		{"and something always exists", map[string]string{}, defaultEditor},
		{"blank does not count", map[string]string{"EDITOR": "   "}, defaultEditor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &app{env: Env{Getenv: func(name string) string { return tt.env[name] }}}

			require.Equal(t, tt.want, a.editorCommand())
		})
	}
}

func TestAnEditorThatFailsIsReported(t *testing.T) {
	t.Parallel()

	r := commentable(t)

	got := isuIn(t, r.Dir(), map[string]string{"EDITOR": "isu-no-such-editor"},
		"comment", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "isu-no-such-editor")
}

func TestAnEditorThatWritesNothingIsAnEmptyComment(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the editor stand-in is a shell script; CI is linux and macos")
	}

	r := commentable(t)

	// Quitting without saving is how a person says never mind.
	got := isuIn(t, r.Dir(), map[string]string{"EDITOR": writeEditor(t, "")},
		"comment", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "nothing to say")
}

func TestSlugifyMakesAFileNameOutOfAPersonsName(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"isu tester", "isu-tester"},
		{"Dmitry G.", "dmitry-g"},
		{"a  b", "a-b"},
		{"Ada Lovelace-Byron", "ada-lovelace-byron"},
		{"???", "anon"},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, slugify(tt.in))
	}
}

// writeEditor puts a script where $EDITOR goes: it writes the given text into
// the file it is handed and exits, which is what an editor does.
func writeEditor(t *testing.T, text string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "editor")

	script := "#!/bin/sh\nprintf '%s' " + shellQuote(text) + " > \"$1\"\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700)) //nolint:gosec // a test's own script

	return path
}

func shellQuote(s string) string {
	var out []byte

	out = append(out, '\'')

	for _, b := range []byte(s) {
		if b == '\'' {
			out = append(out, `'\''`...)

			continue
		}

		out = append(out, b)
	}

	return string(append(out, '\''))
}
