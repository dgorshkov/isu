package gitx_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/gitx"
)

// M4 is the first milestone that writes. Everything up to it read a repository,
// so gitx grew only readers; the wrappers here are the other half.
//
// They are bound to the fixture the way production binds — gitx.New with no
// options — so that the identity and the configuration a commit is written
// under are the repository's own, which is the whole reason PLAN.md §0 shells
// out to git rather than linking a reimplementation of it.

// open binds a Git to a repository the harness built, exactly as a command
// would.
func open(t *testing.T, dir string) *gitx.Git {
	t.Helper()

	g, err := gitx.New(dir)
	require.NoError(t, err)

	return g
}

func TestBuildTreeWritesWithoutTouchingTheWorkingTree(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa")
	g := open(t, r.Dir())
	before := r.ReadFile("issues/ISU-aaaaaa/README.md")

	tree, err := g.BuildTree(t.Context(), gittest.DefaultBranch, []gitx.TreeEdit{{
		Path: "issues/ISU-aaaaaa/README.md",
		Blob: []byte("rewritten\n"),
	}})
	require.NoError(t, err)

	commit, err := g.CommitTree(t.Context(), tree, []string{r.Head()}, "rewrite\n")
	require.NoError(t, err)
	require.NoError(t, g.UpdateRef(t.Context(), "refs/heads/side", commit, gitx.ZeroOID))

	blob, err := g.Show(t.Context(), "side:issues/ISU-aaaaaa/README.md")
	require.NoError(t, err)
	require.Equal(t, "rewritten\n", string(blob))

	// Nothing under the user's hands moved: not the file, not HEAD, and not the
	// index. That is the whole reason M4 builds commits this way rather than
	// checking a branch out to write one line into it — a claim must work from
	// a dirty tree, and a claim that loses its race must leave nothing behind.
	require.Equal(t, before, r.ReadFile("issues/ISU-aaaaaa/README.md"))
	require.Equal(t, gittest.DefaultBranch, r.Git("rev-parse", "--abbrev-ref", "HEAD"))
	require.Empty(t, r.Git("status", "--porcelain"))
}

func TestBuildTreeAddsAPathThatIsNotThereYet(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).File("go.mod", "module x\n").Commit("first")
	g := open(t, r.Dir())

	tree, err := g.BuildTree(t.Context(), gittest.DefaultBranch, []gitx.TreeEdit{{
		Path: "issues/ISU-bbbbbb/README.md",
		Blob: []byte("new\n"),
	}})
	require.NoError(t, err)

	commit, err := g.CommitTree(t.Context(), tree, []string{r.Head()}, "add\n")
	require.NoError(t, err)
	require.NoError(t, g.UpdateRef(t.Context(), "refs/heads/side", commit, gitx.ZeroOID))

	blob, err := g.Show(t.Context(), "side:issues/ISU-bbbbbb/README.md")
	require.NoError(t, err)
	require.Equal(t, "new\n", string(blob))
}

func TestBuildTreeFromNothingIsTheFirstCommit(t *testing.T) {
	t.Parallel()

	r := gittest.New(t)
	g := open(t, r.Dir())

	tree, err := g.BuildTree(t.Context(), "", []gitx.TreeEdit{{
		Path: "issues/ISU-cccccc/README.md",
		Blob: []byte("first\n"),
	}})
	require.NoError(t, err)

	commit, err := g.CommitTree(t.Context(), tree, nil, "root\n")
	require.NoError(t, err)
	require.NoError(t, g.UpdateRef(t.Context(), "refs/heads/"+gittest.DefaultBranch, commit, ""))

	blob, err := g.Show(t.Context(), gittest.DefaultBranch+":issues/ISU-cccccc/README.md")
	require.NoError(t, err)
	require.Equal(t, "first\n", string(blob))
}

func TestBuildTreeReportsABaseThatDoesNotResolve(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa")
	g := open(t, r.Dir())

	_, err := g.BuildTree(t.Context(), "no-such-ref", []gitx.TreeEdit{{
		Path: "a.md", Blob: []byte("x"),
	}})
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)
}

func TestBuildTreeKeepsTheModeItIsGiven(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).File("go.mod", "module x\n").Commit("first")
	g := open(t, r.Dir())

	tree, err := g.BuildTree(t.Context(), gittest.DefaultBranch, []gitx.TreeEdit{
		{Path: "run.sh", Blob: []byte("#!/bin/sh\n"), Mode: "100755"},
	})
	require.NoError(t, err)

	entries, err := g.LsTree(t.Context(), tree)
	require.NoError(t, err)

	modes := map[string]string{}
	for _, e := range entries {
		modes[e.Path] = e.Mode
	}

	require.Equal(t, "100755", modes["run.sh"])
	require.Equal(t, "100644", modes["go.mod"])
}

func TestUpdateRefRefusesToOverwriteWhatItWasToldWasThere(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa").Branch("side")
	g := open(t, r.Dir())

	// ZeroOID means "must not exist". side does, so this is refused — which is
	// how `isu claim` finds out it lost a race locally before it ever pushes.
	require.Error(t, g.UpdateRef(t.Context(), "refs/heads/side", r.Head(), gitx.ZeroOID))
}

func TestDeleteRefRemovesABranch(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa").Branch("side")
	g := open(t, r.Dir())

	require.NoError(t, g.DeleteRef(t.Context(), "refs/heads/side"))
	require.NotContains(t, r.Branches(), "side")
}

func TestHashObjectWritesABlobAndReturnsItsID(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).File("go.mod", "module x\n").Commit("first")
	g := open(t, r.Dir())

	oid, err := g.HashObject(t.Context(), []byte("hello\n"))
	require.NoError(t, err)
	require.Len(t, oid, 40)

	blob, err := g.Show(t.Context(), oid)
	require.NoError(t, err)
	require.Equal(t, "hello\n", string(blob))
}

func TestAddAndCommitWriteThroughTheWorkingTree(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa")
	g := open(t, r.Dir())

	r.WriteFile("issues/ISU-aaaaaa/README.md", "edited\n")
	require.NoError(t, g.Add(t.Context(), "issues/ISU-aaaaaa/README.md"))

	oid, err := g.Commit(t.Context(), "edit ISU-aaaaaa\n\nIsu-Resolves: ISU-aaaaaa\n")
	require.NoError(t, err)
	require.Equal(t, r.Head(), oid)

	require.Equal(t, "edit ISU-aaaaaa", r.Git("log", "-1", "--format=%s"))
	require.Contains(t, r.Git("log", "-1", "--format=%b"), "Isu-Resolves: ISU-aaaaaa")
}

func TestCommitRefusesAnEmptyIndex(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa")
	g := open(t, r.Dir())

	// A command that meant to change a file and did not must say so rather than
	// write an empty commit somebody has to explain later.
	_, err := g.Commit(t.Context(), "nothing changed\n")
	require.Error(t, err)
}

func TestCurrentBranchNamesTheBranchAndSaysNothingWhenDetached(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa")
	g := open(t, r.Dir())

	branch, err := g.CurrentBranch(t.Context())
	require.NoError(t, err)
	require.Equal(t, gittest.DefaultBranch, branch)

	r.Checkout(r.Head())

	branch, err = g.CurrentBranch(t.Context())
	require.NoError(t, err)
	require.Empty(t, branch, "a detached HEAD is on no branch, which is not a failure")
}

func TestCurrentBranchOnARepositoryWithNoCommitsNamesTheUnbornBranch(t *testing.T) {
	t.Parallel()

	r := gittest.New(t)
	g := open(t, r.Dir())

	branch, err := g.CurrentBranch(t.Context())
	require.NoError(t, err)
	require.Equal(t, gittest.DefaultBranch, branch)
}

func TestSwitchCreatesAndMoves(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa")
	g := open(t, r.Dir())

	require.NoError(t, g.Switch(t.Context(), "report/ISU-aaaaaa", true))
	require.Equal(t, "report/ISU-aaaaaa", r.Git("rev-parse", "--abbrev-ref", "HEAD"))

	require.NoError(t, g.Switch(t.Context(), gittest.DefaultBranch, false))
	require.Equal(t, gittest.DefaultBranch, r.Git("rev-parse", "--abbrev-ref", "HEAD"))

	require.Error(t, g.Switch(t.Context(), "report/ISU-aaaaaa", true),
		"creating a branch that is already there is a failure, not a switch")
}

func TestRemotesListsThemAndIsEmptyWithoutOne(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa")
	g := open(t, r.Dir())

	remotes, err := g.Remotes(t.Context())
	require.NoError(t, err)
	require.Empty(t, remotes)

	r.WithRemote()

	remotes, err = g.Remotes(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"origin"}, remotes)
}

func TestFetchUpdatesRemoteTrackingRefs(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa").WithRemote()
	g := open(t, r.Dir())

	// Throw the remote-tracking ref away, so that finding it again is evidence
	// the fetch did something rather than evidence the push already had.
	r.Git("update-ref", "-d", "refs/remotes/origin/"+gittest.DefaultBranch)

	require.NoError(t, g.Fetch(t.Context(), "origin"))

	refs, err := g.ForEachRef(t.Context(), "refs/remotes/")
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, "refs/remotes/origin/"+gittest.DefaultBranch, refs[0].Name)
}

func TestFetchReportsARemoteThatDoesNotAnswer(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).Issue("ISU-aaaaaa").Commit("add ISU-aaaaaa").WithRemote().DetachRemote()
	g := open(t, r.Dir())

	require.Error(t, g.Fetch(t.Context(), "origin"))
}

func TestZeroOIDIsFortyZeroes(t *testing.T) {
	t.Parallel()

	require.Equal(t, strings.Repeat("0", 40), gitx.ZeroOID)
}
