package repo_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/repo"
)

// What the loader does with the things under issues/ that are not issues, and
// with a git that fails.
//
// Both were reachable and untested. The first half matters because the board
// path and the single-ref path make the same judgements twice, in two
// functions, and only one copy was exercised: LoadRef refused a folder that
// cannot be an id, and the branch side of LoadBoard did the same thing with
// nothing watching. Two implementations of one rule, one test between them, is
// how they drift.
//
// The second half matters because every one of those error returns is the
// message a user sees when their repository is not what isu expected, and an
// untested error path is a message nobody has ever read.

// A folder under issues/ whose name cannot be an id is reported rather than
// skipped, wherever it appears. Trunk's copy of that rule had a test; the
// branch's did not.
func TestABranchThatAddsAnIllegalFolderNameIsReported(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("wip").Checkout("wip").
		File("issues/ISU 7f3akq/README.md", "---\nschema: 1\nid: ISU-7f3akq\n---\n").
		Commit("a folder whose name is not an id").
		Checkout(gittest.DefaultBranch)

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	onBranch := board.Refs["refs/heads/wip"]

	require.Equal(t, []string{"ISU-7f3akq"}, onBranch.IDs(),
		"the folder somebody misnamed is not one of the issues")
	require.Len(t, onBranch.Broken, 1, "and it is not silently skipped either")
	require.Equal(t, "ISU 7f3akq", onBranch.Broken[0].ID)
	require.ErrorContains(t, onBranch.Broken[0].Err, "cannot be an issue id")
}

// `issues/README.md` explaining the directory to a newcomer is not an issue.
// The loader's own comment says so; nothing asserted it.
func TestTheIssuesDirectoryReadmeIsNotAnIssue(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").
		File("issues/README.md", "# Issues\n\nOne folder per issue.\n").
		Commit("an explainer beside the issues")

	loader := open(t, r)

	set, err := loader.LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, []string{"ISU-7f3akq"}, set.IDs())
	require.Empty(t, set.Broken, "it is not an issue, and it is not a broken one either")

	board, err := loader.LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)
	require.Equal(t, []string{"ISU-7f3akq"}, board.Trunk.IDs())
	require.Empty(t, board.Trunk.Broken)

	tree, err := loader.LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"ISU-7f3akq"}, tree.IDs())
	require.Empty(t, tree.Broken, "all three loaders agree about what is not an issue")
}

// A branch that changes an attachment or a comment has changed something under
// issues/ without changing an issue. The diff names it; the loader must not
// mistake it for one.
func TestABranchThatChangesOnlyAnAttachmentChangesNoIssue(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq", gittest.Attachment("repro.har", "{}\n")).
		Commit("report ISU-7f3akq with a recording").
		Branch("wip").Checkout("wip").
		File("issues/ISU-7f3akq/repro.har", "{\"log\": []}\n").
		File("issues/ISU-7f3akq/comments/2026-08-29-sam-01.md", "Still broken.\n").
		Commit("a better recording, and a comment").
		Checkout(gittest.DefaultBranch)

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	onTrunk, _ := board.Trunk.Get("ISU-7f3akq")
	onBranch, _ := board.Refs["refs/heads/wip"].Get("ISU-7f3akq")

	require.Same(t, onTrunk, onBranch,
		"the issue's own file did not change, so neither did the issue")
	require.Empty(t, board.Changed["refs/heads/wip"],
		"the ref differs from trunk, and not about any issue")
}

// Two refs carrying the same unreadable file report it once each, from one
// decode. The cache holds what failed for the same reason it holds what
// succeeded: a file is read and parsed once however many refs share it.
func TestTwoRefsSharingOneUnreadableFileEachReportIt(t *testing.T) {
	const broken = "---\nid: ISU-40b1cc\nschema: 99\n---\n"

	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("one").Checkout("one").
		File("issues/ISU-40b1cc/README.md", broken).Commit("a file this build cannot read").
		Checkout(gittest.DefaultBranch).
		Branch("two").Checkout("two").
		File("issues/ISU-40b1cc/README.md", broken).Commit("the same file again").
		Checkout(gittest.DefaultBranch)

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	for _, ref := range []string{"refs/heads/one", "refs/heads/two"} {
		set := board.Refs[ref]
		require.Len(t, set.Broken, 1, "%s", ref)
		require.Equal(t, "ISU-40b1cc", set.Broken[0].ID, "%s", ref)
		require.ErrorContains(t, set.Broken[0].Err, "99", "%s: the schema it cannot read", ref)
	}
}

// A ref that fixes one broken issue leaves the other one broken. Forgetting
// what was wrong with an issue is per-issue, and a loader that dropped the lot
// would hide a file nobody has fixed.
func TestFixingOneBrokenIssueLeavesTheOtherBroken(t *testing.T) {
	r := gittest.New(t).
		File("issues/ISU-7f3akq/README.md", "no frontmatter here\n").
		File("issues/ISU-40b1cc/README.md", "nor here\n").
		Commit("two unreadable issues").
		Branch("fix").Checkout("fix").
		Issue("ISU-7f3akq").Commit("fix one of them").
		Checkout(gittest.DefaultBranch)

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	require.Len(t, board.Trunk.Broken, 2)

	fixed := board.Refs["refs/heads/fix"]
	require.Equal(t, []string{"ISU-7f3akq"}, fixed.IDs())
	require.Len(t, fixed.Broken, 1, "the one nobody fixed is still reported")
	require.Equal(t, "ISU-40b1cc", fixed.Broken[0].ID)
}

// Every loader reports a git that fails rather than returning an empty answer.
//
// A board with no issues in it and a board that could not be read look the same
// to everything downstream, so the difference has to be the error — and an
// error return nothing has ever taken is a claim, not a behaviour.
func TestEveryLoaderReportsAGitThatFails(t *testing.T) {
	r := gittest.New(t).Issue("ISU-7f3akq").Commit("report ISU-7f3akq")

	loader := brokenGit(t, r)
	ctx := t.Context()

	t.Run("LoadRef", func(t *testing.T) {
		_, err := loader.LoadRef(ctx, gittest.DefaultBranch)
		require.Error(t, err)
	})

	t.Run("LoadBoard", func(t *testing.T) {
		_, err := loader.LoadBoard(ctx, repo.BoardSpec{Trunk: gittest.DefaultBranch})
		require.Error(t, err)
	})

	t.Run("LoadHistory", func(t *testing.T) {
		_, err := loader.LoadHistory(ctx, gittest.DefaultBranch)
		require.Error(t, err)
	})

	t.Run("LoadWorktree", func(t *testing.T) {
		_, err := loader.LoadWorktree(ctx)
		require.Error(t, err)
	})

	t.Run("LoadFirstCommits", func(t *testing.T) {
		_, err := loader.LoadFirstCommits(ctx, gittest.DefaultBranch, []string{"refs/heads/x"})
		require.Error(t, err)
	})
}

// The board's later phases fail too, not just the first call it makes. Each of
// them is a separate return, and a test that only ever broke the first would
// leave the rest as unread claims.
func TestTheBoardReportsAFailureAtEveryPhase(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("wip").Checkout("wip").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("resolve it").
		Checkout(gittest.DefaultBranch)

	// A ref that resolves and a trunk that does not: the ls-tree succeeds, and
	// the diff-tree against a revision that is not there is what fails.
	_, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{
		Trunk: gittest.DefaultBranch, Patterns: []string{"refs/heads/", "refs/nowhere/"},
	})
	require.NoError(t, err, "a pattern matching nothing is not a failure")

	_, err = open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: "no-such-ref"})
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)
	require.ErrorContains(t, err, "no-such-ref")
}

// An unborn HEAD is an empty answer and every other name that does not resolve
// is a mistake worth reporting — asserted for each loader, because each one
// makes that judgement separately.
func TestAnUnbornHeadIsEmptyAndATypoIsAnError(t *testing.T) {
	r := gittest.New(t)
	loader := open(t, r)
	ctx := t.Context()

	set, err := loader.LoadRef(ctx, "HEAD")
	require.NoError(t, err)
	require.Empty(t, set.IDs())

	history, err := loader.LoadHistory(ctx, "HEAD")
	require.NoError(t, err)
	require.Empty(t, history)

	_, err = loader.LoadRef(ctx, "typo")
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)

	_, err = loader.LoadHistory(ctx, "typo")
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)
}

// brokenGit is the repository read through a git that always fails.
//
// It is the harness's own shim technique, pointed the other way: gitx already
// takes the binary as an option so that a missing git can be tested, and a git
// that runs and exits non-zero is the same door.
func brokenGit(t *testing.T, r *gittest.Repo) *repo.Repo {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script; CI is linux and macos")
	}

	shim := filepath.Join(t.TempDir(), "git")
	require.NoError(t, os.WriteFile(shim,
		[]byte("#!/bin/sh\necho 'fatal: this git refuses' >&2\nexit 1\n"), 0o700))

	loader, err := repo.Open(r.Dir(), repo.WithGit(gitx.WithBinary(shim)))
	require.NoError(t, err, "the binary is there; it just fails when it runs")

	return loader
}

// A repository path that is not a directory is refused before any command runs.
func TestOpeningSomethingThatIsNotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a-file")
	require.NoError(t, os.WriteFile(file, []byte("not a repository\n"), 0o600))

	_, err := repo.Open(file)

	require.Error(t, err)
	require.ErrorContains(t, err, "directory")
	require.False(t, errors.Is(err, gitx.ErrNotFound), "git is installed; this is not it")
}
