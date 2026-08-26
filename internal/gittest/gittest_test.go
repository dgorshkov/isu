package gittest_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

func TestNewStartsAnEmptyRepositoryOnTheDefaultBranch(t *testing.T) {
	r := gittest.New(t)

	require.Equal(t, gittest.DefaultBranch, r.Git("branch", "--show-current"))
	require.Equal(t, "0", r.Git("rev-list", "--count", "--all"),
		"a fresh repository has no commits, so M2-S2 can load from an empty ref")
	require.Empty(t, r.Git("status", "--porcelain"))
	require.DirExists(t, filepath.Join(r.Dir(), ".git"))
}

// The harness must not read the developer's own git configuration, or a test
// passes on one machine and fails on the next because of a global template
// directory, a default branch name or a signing key.
func TestNewIgnoresTheAmbientGitConfiguration(t *testing.T) {
	r := gittest.New(t)

	require.Equal(t, "isu tester", r.Git("config", "--get", "user.name"))
	require.Equal(t, "false", r.Git("config", "--get", "commit.gpgsign"),
		"a developer who signs their commits must not sign the fixtures")

	_, err := r.Try("config", "--get", "user.signingkey")
	require.Error(t, err, "no signing key reaches the repository from the developer's config")
}

func TestIssueWritesAnIssueFolderAndStagesIt(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq",
		gittest.Title("Login retries"),
		gittest.Type("bug"),
		gittest.State("open"),
		gittest.Owner("dmitry"),
		gittest.Created("2026-08-24"),
		gittest.Priority("p1"),
		gittest.Field("repro", "log in twice"),
		gittest.Body("Free-form markdown body.\n"),
		gittest.Comment("2026-08-24-support-01.md", "Seen it too.\n"),
		gittest.Attachment("repro.har", "{}\n"),
	)

	readme := r.ReadFile("issues/AR-7f3akq/README.md")
	require.True(t, strings.HasPrefix(readme, "---\n"), "frontmatter opens the file")
	for _, line := range []string{
		"schema: 1",
		"id: AR-7f3akq",
		"title: Login retries",
		"type: bug",
		"state: open",
		"owner: dmitry",
		"created: 2026-08-24",
		"priority: p1",
		"repro: log in twice",
	} {
		require.Contains(t, readme, line+"\n")
	}
	require.True(t, strings.HasSuffix(readme, "Free-form markdown body.\n"))

	require.Equal(t, "Seen it too.\n",
		r.ReadFile("issues/AR-7f3akq/comments/2026-08-24-support-01.md"))
	require.Equal(t, "{}\n", r.ReadFile("issues/AR-7f3akq/repro.har"))

	// Written and staged, but not committed: what is on disk and what is in the
	// index are different questions and M2-S3 tests both.
	require.Empty(t, r.Git("diff", "--name-only"), "the issue is staged")
	require.Equal(t,
		"issues/AR-7f3akq/README.md\n"+
			"issues/AR-7f3akq/comments/2026-08-24-support-01.md\n"+
			"issues/AR-7f3akq/repro.har",
		r.Git("diff", "--cached", "--name-only"))
}

func TestIssueDefaultsToAValidOpenChore(t *testing.T) {
	r := gittest.New(t).Issue("AR-40b1cc")

	readme := r.ReadFile("issues/AR-40b1cc/README.md")
	require.Contains(t, readme, "schema: 1\n")
	require.Contains(t, readme, "id: AR-40b1cc\n")
	require.Contains(t, readme, "type: chore\n", "chore is the only type needing no extra field")
	require.Contains(t, readme, "state: open\n")
	require.Contains(t, readme, "created: "+time.Now().UTC().Format(time.DateOnly)+"\n")
}

// An epic must not declare a state, and its children point at it. Both are
// frontmatter shapes the derivation layer folds over in M3, so the harness has
// to be able to write them.
func TestIssueWritesAnEpicAndAChildThatPointsAtIt(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-40b1cc",
			gittest.Type("epic"),
			gittest.Without("state"),
			gittest.Without("no-such-key"),
		).
		Issue("AR-7f3akq",
			gittest.Parent("AR-40b1cc"),
			gittest.BlockedBy("AR-39ka2p", "AR-4820qx"),
			gittest.Comment("2026-08-24-support-01", "The extension is optional.\n"),
		)

	epic := r.ReadFile("issues/AR-40b1cc/README.md")
	require.NotContains(t, epic, "state:", "an epic's status is the fold over its children")
	require.Contains(t, epic, "type: epic\n")
	require.True(t, strings.HasSuffix(epic, "---\n"), "an issue may have no body at all")

	child := r.ReadFile("issues/AR-7f3akq/README.md")
	require.Contains(t, child, "parent: AR-40b1cc\n")
	require.Contains(t, child, "blocked_by: AR-39ka2p, AR-4820qx\n")

	require.Equal(t, "The extension is optional.\n",
		r.ReadFile("issues/AR-7f3akq/comments/2026-08-24-support-01.md"))
}

func TestBranchesIsEmptyUntilTheFirstCommit(t *testing.T) {
	r := gittest.New(t)

	require.Empty(t, r.Branches(), "an unborn branch is not a branch yet")

	r.Issue("AR-7f3akq").Commit("add AR-7f3akq")
	require.Equal(t, []string{gittest.DefaultBranch}, r.Branches())
}

func TestCommitRecordsExactlyWhatWasStaged(t *testing.T) {
	r := gittest.New(t).
		File("README.md", "# fixture\n").
		Issue("AR-7f3akq").
		Commit("add AR-7f3akq")

	require.Equal(t, "add AR-7f3akq", r.Git("log", "--format=%s"))
	require.Equal(t, "1", r.Git("rev-list", "--count", "HEAD"))
	require.Equal(t,
		"README.md\nissues/AR-7f3akq/README.md",
		r.Git("ls-tree", "-r", "--name-only", "HEAD"))
	require.Empty(t, r.Git("status", "--porcelain"), "nothing is left behind uncommitted")
	require.Equal(t, r.Git("rev-parse", "HEAD"), r.Head())
}

func TestBranchCreatesWithoutSwitchingAndCheckoutSwitches(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq")

	trunk := r.Head()
	require.Equal(t, gittest.DefaultBranch, r.Git("branch", "--show-current"),
		"Branch creates a ref; Checkout is what moves")

	r.Checkout("isu/AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).
		Commit("resolve AR-7f3akq")

	require.Equal(t, "isu/AR-7f3akq", r.Git("branch", "--show-current"))
	require.Contains(t, r.ReadFile("issues/AR-7f3akq/README.md"), "state: resolved\n")

	r.Checkout(gittest.DefaultBranch)
	require.Equal(t, trunk, r.Head(), "trunk did not move")
	require.Contains(t, r.ReadFile("issues/AR-7f3akq/README.md"), "state: open\n")
}

func TestMergeMakesAMergeCommitWithTwoParents(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq").
		Checkout(gittest.DefaultBranch).
		Merge("isu/AR-7f3akq")

	parents := strings.Fields(r.Git("rev-list", "--parents", "-n", "1", "HEAD"))
	require.Len(t, parents, 3, "a merge commit is its own sha plus two parents")

	require.Contains(t, r.ReadFile("issues/AR-7f3akq/README.md"), "state: resolved\n")
	require.Equal(t, "0", r.Git("rev-list", "--count", "isu/AR-7f3akq", "--not", "HEAD"),
		"every commit on the branch is now reachable from trunk")
}

// GitLab squashes by default and GitHub is often configured to, so the whole
// derivation layer has to work when the branch's commits never reach trunk.
func TestSquashMergeLandsOneCommitThatDoesNotReachTheBranch(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("wip").
		File("fix.go", "package fix\n").Commit("fix it").
		Checkout(gittest.DefaultBranch).
		SquashMerge("isu/AR-7f3akq", "resolve AR-7f3akq")

	require.Equal(t, "resolve AR-7f3akq\nadd AR-7f3akq", r.Git("log", "--format=%s"))

	parents := strings.Fields(r.Git("rev-list", "--parents", "-n", "1", "HEAD"))
	require.Len(t, parents, 2, "a squash merge has one parent, not two")

	require.Contains(t, r.ReadFile("issues/AR-7f3akq/README.md"), "state: resolved\n")
	require.FileExists(t, filepath.Join(r.Dir(), "fix.go"))

	_, err := r.Try("merge-base", "--is-ancestor", "isu/AR-7f3akq", "HEAD")
	require.Error(t, err, "the branch's commits are not reachable from trunk after a squash")
}

func TestRevertUndoesACommit(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq").
		Revert("HEAD")

	require.Contains(t, r.ReadFile("issues/AR-7f3akq/README.md"), "state: open\n",
		"reverting the resolve puts the issue back to open, which is `reopened` in M3")
	require.Equal(t, "3", r.Git("rev-list", "--count", "HEAD"),
		"a revert is a new commit, not a rewrite")
}

func TestWithRemotePublishesTheCurrentBranch(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		WithRemote()

	require.NotEmpty(t, r.RemoteDir())
	require.Contains(t, r.Git("ls-remote", "origin"), r.Head())
	require.Equal(t, "origin/"+gittest.DefaultBranch,
		r.Git("rev-parse", "--abbrev-ref", gittest.DefaultBranch+"@{upstream}"),
		"the branch tracks the remote, so a push in a later story needs no arguments")

	r.Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq")
	r.Git("push", "--quiet", "origin", "HEAD")
	require.Contains(t, r.Git("ls-remote", "origin"), r.Head())
}

// M9-S3 runs a whole session with the remote unreachable. Removing the remote
// would model something else entirely — a repository that never had one — so
// the remote stays configured and stops answering.
func TestDetachRemoteLeavesTheRemoteConfiguredAndUnreachable(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		WithRemote().
		DetachRemote()

	require.NotEmpty(t, r.Git("config", "--get", "remote.origin.url"),
		"the repository still believes it has a remote")

	_, err := r.Try("fetch", "origin")
	require.Error(t, err, "fetching a detached remote must fail")
}

func TestBackdateMovesTheClockForLaterCommits(t *testing.T) {
	r := gittest.New(t).
		Backdate(30).
		Issue("AR-7f3akq").Commit("add AR-7f3akq")

	old := commitTime(t, r, "HEAD")
	require.WithinDuration(t, time.Now().AddDate(0, 0, -30), old, time.Hour)

	r.Backdate(0).Issue("AR-40b1cc").Commit("add AR-40b1cc")

	require.WithinDuration(t, time.Now(), commitTime(t, r, "HEAD"), time.Hour)
	require.True(t, commitTime(t, r, "HEAD~1").Before(commitTime(t, r, "HEAD")),
		"history stays in order")
}

func commitTime(t *testing.T, r *gittest.Repo, ref string) time.Time {
	t.Helper()

	secs, err := strconv.ParseInt(r.Git("log", "-1", "--format=%at", ref), 10, 64)
	require.NoError(t, err)

	return time.Unix(secs, 0)
}

// The done-when for M0-S4: two branches and a squash merge, in five lines.
func TestFiveLinesProduceTwoBranchesAndASquashMerge(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq").
		Checkout(gittest.DefaultBranch).SquashMerge("isu/AR-7f3akq", "resolve AR-7f3akq")

	require.Equal(t, "resolve AR-7f3akq\nadd AR-7f3akq", r.Git("log", "--format=%s"))
	require.ElementsMatch(t, []string{gittest.DefaultBranch, "isu/AR-7f3akq"}, r.Branches())
}
