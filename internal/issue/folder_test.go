package issue_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
)

// issueDir is where the harness puts the issue it was asked for.
func issueDir(r *gittest.Repo, id string) string {
	return filepath.Join(r.Dir(), "issues", id)
}

func TestLoadReadsTheWholeFolder(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).
		Issue("ISU-7f3akq",
			gittest.Title("Login retries stop after the third attempt"),
			gittest.Type("bug"),
			gittest.Owner("dmitry"),
			gittest.Created("2026-08-24"),
			gittest.Field("repro", "POST /session five times"),
			gittest.Field("jira_key", "PROJ-1234"),
			gittest.Body("The retry counter is reset by the wrong branch.\n"),
			gittest.Attachment("repro.har", "{}"),
			gittest.Attachment("screenshot.png", "\x89PNG"),
			gittest.Comment("2026-08-24-support-01", "Reproduced on staging.\n"),
			gittest.Comment("2026-08-24-support-02", "And on production.\n"),
			gittest.Comment("2026-08-25-dmitry-01", "Fix is in isu/ISU-7f3akq.\n"),
		).
		Commit("add ISU-7f3akq")

	f, err := issue.Load(issueDir(r, "ISU-7f3akq"))
	require.NoError(t, err)

	require.Equal(t, "ISU-7f3akq", f.Issue.ID)
	require.Equal(t, "ISU-7f3akq", f.Issue.Folder)
	require.Equal(t, issue.TypeBug, f.Issue.Type)
	require.Equal(t, "The retry counter is reset by the wrong branch.\n", f.Issue.Body)
	require.NoError(t, f.Issue.Validate())

	require.Equal(t, []string{"repro.har", "screenshot.png"}, f.Attachments)

	require.Len(t, f.Comments, 3)
	require.Equal(t, []string{
		"2026-08-24-support-01.md",
		"2026-08-24-support-02.md",
		"2026-08-25-dmitry-01.md",
	}, []string{f.Comments[0].Name, f.Comments[1].Name, f.Comments[2].Name})
	require.Equal(t, "Reproduced on staging.\n", f.Comments[0].Body)
	require.Equal(t, "Fix is in isu/ISU-7f3akq.\n", f.Comments[2].Body)
}

func TestLoadAFolderWithNothingButAReadme(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-7f3akq").Commit("add ISU-7f3akq")

	f, err := issue.Load(issueDir(r, "ISU-7f3akq"))
	require.NoError(t, err)
	require.Empty(t, f.Attachments)
	require.Empty(t, f.Comments)
}

// The property this story exists for. A pull request that touches an issue it
// did not mean to touch is a pull request nobody reads carefully.
func TestWriteAnUnmodifiedIssueIsAZeroLengthDiff(t *testing.T) {
	t.Parallel()

	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join("testdata", "issues", name))
			require.NoError(t, err)

			// The fixture's id is whatever the fixture says, and the folder has
			// to agree with it or this is a test about something else.
			doc, err := issue.Parse(body)
			require.NoError(t, err)
			id, ok := doc.Get("id")
			require.True(t, ok, "every fixture declares an id")

			r := gittest.New(t).
				File("issues/"+id+"/README.md", string(body)).
				Commit("add " + id)

			f, err := issue.Load(issueDir(r, id))
			require.NoError(t, err)
			require.NoError(t, f.Write())

			r.Git("diff", "--exit-code")
			r.Git("diff", "--cached", "--exit-code")
		})
	}
}

func TestWriteAModifiedIssueTouchesOnlyTheLineThatChanged(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).
		Issue("ISU-7f3akq",
			gittest.Field("jira_key", "PROJ-1234"),
			gittest.Body("The body.\n"),
		).
		Commit("add ISU-7f3akq")

	f, err := issue.Load(issueDir(r, "ISU-7f3akq"))
	require.NoError(t, err)

	f.Issue.State = issue.StateResolved
	require.NoError(t, f.Write())

	diff := r.Git("diff", "--unified=0", "--", "issues/ISU-7f3akq/README.md")

	var changed []string
	for _, line := range strings.Split(diff, "\n") {
		if (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")) &&
			!strings.HasPrefix(line, "+++") && !strings.HasPrefix(line, "---") {
			changed = append(changed, line)
		}
	}

	require.Equal(t, []string{"-state: open", "+state: resolved"}, changed)
}

func TestWriteCreatesTheFolder(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "issues", "ISU-7f3akq")

	f := &issue.Folder{
		Path: dir,
		Issue: &issue.Issue{
			Schema:  issue.CurrentSchema,
			ID:      "ISU-7f3akq",
			Title:   "Written from nothing",
			Type:    issue.TypeChore,
			State:   issue.StateOpen,
			Owner:   "dmitry",
			Created: mustDate(t, "2026-08-24"),
			Body:    "The body.\n",
		},
	}
	require.NoError(t, f.Write())

	again, err := issue.Load(dir)
	require.NoError(t, err)
	require.NoError(t, again.Issue.Validate())

	written, err := os.ReadFile(filepath.Join(dir, issue.ReadmeName))
	require.NoError(t, err)
	require.Equal(t, strings.Join([]string{
		"---",
		"schema: 1",
		"id: ISU-7f3akq",
		"title: Written from nothing",
		"type: chore",
		"state: open",
		"owner: dmitry",
		"created: 2026-08-24",
		"---",
		"The body.\n",
	}, "\n"), string(written),
		"a new issue writes its frontmatter in the order the schema documents it")
}

// Clearing an optional field removes its line rather than leaving an empty one
// behind, which is what a reader would otherwise have to treat as set.
func TestWriteRemovesAClearedOptionalField(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "ISU-7f3akq")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, issue.ReadmeName), []byte(
		frontmatter("schema: 1", "id: ISU-7f3akq", "priority: p0", "parent: ISU-40b1cc")),
		0o644))

	f, err := issue.Load(dir)
	require.NoError(t, err)

	f.Issue.Priority = ""
	f.Issue.Parent = ""
	f.Issue.Title = "Set from nothing"
	require.NoError(t, f.Write())

	written, err := os.ReadFile(filepath.Join(dir, issue.ReadmeName))
	require.NoError(t, err)
	require.Equal(t,
		frontmatter("schema: 1", "id: ISU-7f3akq", "title: Set from nothing"),
		string(written))
}

// A date the file wrote as a full timestamp is written back as it was, and one
// the caller actually moved is written in the form isu writes.
func TestWriteKeepsTheDateFormTheFileUsed(t *testing.T) {
	t.Parallel()

	i := decode(t, frontmatter("schema: 1", "id: ISU-7f3akq", "created: 2026-07-14T09:12:00Z"))
	require.Equal(t,
		frontmatter("schema: 1", "id: ISU-7f3akq", "created: 2026-07-14T09:12:00Z"),
		string(i.Encode()))

	i.Created = mustDate(t, "2026-08-24")
	require.Equal(t,
		frontmatter("schema: 1", "id: ISU-7f3akq", "created: 2026-08-24"),
		string(i.Encode()))

	// An issue whose date was never set writes no created line at all, and
	// Validate is what reports it rather than the encoder inventing one.
	blank := decode(t, frontmatter("schema: 1", "id: ISU-7f3akq"))
	require.Equal(t, frontmatter("schema: 1", "id: ISU-7f3akq"), string(blank.Encode()))
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(t *testing.T, dir string)
		msg   string
	}{
		{
			name:  "a folder that is not there",
			build: func(*testing.T, string) {},
			msg:   "reading the issue folder",
		},
		{
			name: "a folder that is a file",
			build: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(dir, []byte("not a folder"), 0o644))
			},
			msg: "reading the issue folder",
		},
		{
			name: "a folder with no README.md",
			build: func(t *testing.T, dir string) {
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "repro.har"), nil, 0o644))
			},
			msg: "has no README.md",
		},
		{
			name: "a README.md that is a directory",
			build: func(t *testing.T, dir string) {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, issue.ReadmeName), 0o755))
			},
			msg: "reading the issue",
		},
		{
			name: "a README.md that does not parse",
			build: func(t *testing.T, dir string) {
				writeIssue(t, dir, "no frontmatter here\n")
			},
			msg: "line 1: the file must open with ---",
		},
		{
			name: "a README.md carrying a schema this build does not read",
			build: func(t *testing.T, dir string) {
				writeIssue(t, dir, frontmatter("schema: 2", "id: ISU-7f3akq"))
			},
			msg: "found 2",
		},
		{
			name: "a comments entry that is a file",
			build: func(t *testing.T, dir string) {
				writeIssue(t, dir, frontmatter("schema: 1", "id: ISU-7f3akq"))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, issue.CommentsDir), []byte("oops"), 0o644))
			},
			msg: "reading the comments",
		},
		{
			name: "a comment that is a directory",
			build: func(t *testing.T, dir string) {
				writeIssue(t, dir, frontmatter("schema: 1", "id: ISU-7f3akq"))
				require.NoError(t, os.MkdirAll(
					filepath.Join(dir, issue.CommentsDir, "2026-08-24-support-01.md"), 0o755))
			},
			msg: "reading a comment",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "ISU-7f3akq")
			tc.build(t, dir)

			f, err := issue.Load(dir)
			require.Nil(t, f)
			require.ErrorContains(t, err, tc.msg)
		})
	}
}

// Anything in comments/ that is not a .md file is not a comment. Editors leave
// swap files there and an importer may leave a manifest; neither is something
// somebody said.
func TestLoadIgnoresNonMarkdownInTheCommentsDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "ISU-7f3akq")
	writeIssue(t, dir, frontmatter("schema: 1", "id: ISU-7f3akq"))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, issue.CommentsDir), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, issue.CommentsDir, ".manifest.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, issue.CommentsDir, "2026-08-24-support-01.md"),
		[]byte("Said out loud.\n"), 0o644))

	f, err := issue.Load(dir)
	require.NoError(t, err)
	require.Len(t, f.Comments, 1)
	require.Equal(t, "2026-08-24-support-01.md", f.Comments[0].Name)
}

// A directory beside the README that is not comments/ is not an attachment.
// Nothing in the layout puts one there, so it is not ours to interpret.
func TestLoadIgnoresOtherDirectories(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "ISU-7f3akq")
	writeIssue(t, dir, frontmatter("schema: 1", "id: ISU-7f3akq"))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "scratch"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "repro.har"), []byte("{}"), 0o644))

	f, err := issue.Load(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"repro.har"}, f.Attachments)
}

func TestWriteRejects(t *testing.T) {
	t.Parallel()

	t.Run("a folder whose parent is a file", func(t *testing.T) {
		t.Parallel()

		parent := filepath.Join(t.TempDir(), "issues")
		require.NoError(t, os.WriteFile(parent, []byte("not a folder"), 0o644))

		f := &issue.Folder{Path: filepath.Join(parent, "ISU-7f3akq"), Issue: &issue.Issue{}}
		require.ErrorContains(t, f.Write(), "creating the issue folder")
	})

	t.Run("a README.md that is a directory", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "ISU-7f3akq")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, issue.ReadmeName), 0o755))

		f := &issue.Folder{Path: dir, Issue: &issue.Issue{}}
		require.ErrorContains(t, f.Write(), "writing ")
	})
}

// fixtureNames lists the round-trip fixtures by name.
func fixtureNames(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join("testdata", "issues"))
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func writeIssue(t *testing.T, dir, body string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, issue.ReadmeName), []byte(body), 0o644))
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()

	when, err := time.Parse(time.DateOnly, value)
	require.NoError(t, err)

	return when
}
