package repo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
)

// agreementIssues is the size PLAN.md M2-S3 is done at. The property is cheap
// to hold on three issues and only interesting on five thousand: that is where
// a loader that skipped a folder, followed a link or read a stale blob has
// somewhere to hide.
const agreementIssues = 5000

func TestLoadWorktreeSeesAnUncommittedIssue(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Issue("AR-40b1cc", gittest.Title("Not committed yet"))

	loader := open(t, r)

	committed, err := loader.LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, committed.IDs())

	onDisk, err := loader.LoadWorktree(t.Context())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"AR-7f3akq", "AR-40b1cc"}, onDisk.IDs())
	require.Equal(t, "Not committed yet", mustGet(t, onDisk, "AR-40b1cc").Title,
		"`isu ui` renders what is on disk, edits included")
}

func TestLoadWorktreeSeesAnUncommittedStateFlip(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved"))

	loader := open(t, r)

	committed, err := loader.LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, issue.StateOpen, mustGet(t, committed, "AR-7f3akq").State)

	onDisk, err := loader.LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Equal(t, issue.StateResolved, mustGet(t, onDisk, "AR-7f3akq").State)
}

func TestLoadWorktreeReportsAHalfWrittenIssueRatherThanFailing(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").
		WriteFile("issues/AR-40b1cc/README.md", "---\nid: AR-40b1cc\n").
		Commit("one good issue")

	set, err := open(t, r).LoadWorktree(t.Context())
	require.NoError(t, err, "a half-written issue must not blind the whole board")

	require.Equal(t, []string{"AR-7f3akq"}, set.IDs())
	require.Len(t, set.Broken, 1)
	require.Equal(t, "AR-40b1cc", set.Broken[0].ID)
	require.ErrorContains(t, set.Broken[0].Err, "never closed")
}

func TestLoadWorktreeHonoursGitignore(t *testing.T) {
	r := gittest.New(t).
		File(".gitignore", "issues/AR-40b1cc/\nissues/*/scratch.md\n").
		Issue("AR-7f3akq").
		Commit("one issue and an ignore file").
		WriteFile("issues/AR-40b1cc/README.md",
			"---\nschema: 1\nid: AR-40b1cc\ntitle: ignored\ntype: chore\n"+
				"state: open\nowner: tester\ncreated: 2026-08-24\n---\n").
		WriteFile("issues/AR-7f3akq/scratch.md", "a scratch file\n")

	set, err := open(t, r).LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, set.IDs(),
		"an ignored issue is not tracked, so it is not on the board")
	require.Empty(t, set.Broken)
}

// A file that .gitignore matches but git already tracks is tracked: git's own
// rule, and the loader must not have a different one.
func TestLoadWorktreeKeepsATrackedIssueThatGitignoreAlsoMatches(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		File(".gitignore", "issues/AR-7f3akq/\n").Commit("ignore it after the fact")

	set, err := open(t, r).LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, set.IDs())
}

func TestLoadWorktreeIgnoresEverythingThatIsNotAnIssueReadme(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq",
			gittest.Attachment("repro.har", "{}\n"),
			gittest.Comment("2026-08-24-support-01.md", "Seen it too.\n")).
		File("issues/README.md", "# how this directory works\n").
		File("issues/AR-40b1cc/notes/deep.md", "not a README\n").
		File("docs/AR-39ka2p/README.md", "not under issues/\n").
		Commit("an issue and some decoys")

	set, err := open(t, r).LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, set.IDs())
	require.Empty(t, set.Broken)
}

func TestLoadWorktreeWithNoIssuesDirectory(t *testing.T) {
	r := gittest.New(t).File("README.md", "# a repository\n").Commit("no issues yet")

	set, err := open(t, r).LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Empty(t, set.IDs())
	require.Empty(t, set.Broken)
}

func TestLoadWorktreeOnAnEmptyRepository(t *testing.T) {
	set, err := open(t, gittest.New(t)).LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Empty(t, set.IDs())
}

// The property M2-S3 is done when: on a clean checkout the two loaders agree
// exactly. They are different code reading different sources, and everything
// downstream — the board, the TUI, every check — assumes they cannot disagree.
func TestLoadWorktreeAndLoadRefAgreeOnACleanCheckout(t *testing.T) {
	r := gittest.Generate(t, gittest.Spec{Issues: agreementIssues})
	loader := open(t, r)

	fromRef, err := loader.LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	fromDisk, err := loader.LoadWorktree(t.Context())
	require.NoError(t, err)

	require.Empty(t, fromRef.Broken)
	require.Empty(t, fromDisk.Broken)
	require.Equal(t, agreementIssues, fromRef.Len())
	require.Equal(t, fromRef.IDs(), fromDisk.IDs())

	for _, id := range fromRef.IDs() {
		require.Equal(t, mustGet(t, fromRef, id), mustGet(t, fromDisk, id), "issue %s", id)
	}
}

func TestLoadWorktreeSortsWhatItCouldNotRead(t *testing.T) {
	r := gittest.New(t)
	for _, id := range []string{"AR-zzzzzz", "AR-mmmmmm", "AR-aaaaaa"} {
		r.WriteFile("issues/"+id+"/README.md", "not an issue\n")
	}

	set, err := open(t, r).LoadWorktree(t.Context())
	require.NoError(t, err)

	ids := make([]string, 0, len(set.Broken))
	for _, b := range set.Broken {
		ids = append(ids, b.ID)
	}

	require.Equal(t, []string{"AR-aaaaaa", "AR-mmmmmm", "AR-zzzzzz"}, ids)
}

func TestLoadWorktreeReportsAFolderThatCannotBeAnID(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").
		WriteFile("issues/not an id/README.md", "---\nschema: 1\n---\n")

	set, err := open(t, r).LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, set.IDs())
	require.Len(t, set.Broken, 1)
	require.ErrorContains(t, set.Broken[0].Err, "cannot be an issue id")
}

// Walking the tree is filesystem work; only the ignore list costs a process.
func TestLoadWorktreeSpawnsOneProcess(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Issue("AR-40b1cc").Commit("two issues")
	loader := open(t, r)

	_, err := loader.LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(1), loader.Processes())
}
