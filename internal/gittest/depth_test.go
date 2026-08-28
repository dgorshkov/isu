package gittest_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// Spec.Commits builds the one dimension nothing else in this project measures:
// how long trunk is, which is bounded by the repository's age rather than by
// its backlog.
//
// The benchmarks that use it live in internal/repo and cost tens of seconds, so
// they are not run by `go test ./...`. This is what keeps the generator honest
// between those runs, and it is small on purpose — a couple of hundred commits
// says everything a fixture can be wrong about.
func TestGenerateBuildsTrunkToTheRequestedDepth(t *testing.T) {
	const (
		issues   = 20
		commits  = 200
		touching = 40
	)

	r := gittest.Generate(t, gittest.Spec{
		Issues: issues, Commits: commits, TouchesIssues: touching,
	})

	depth, err := strconv.Atoi(r.Git("rev-list", "--count", gittest.DefaultBranch))
	require.NoError(t, err)
	require.Equal(t, commits, depth, "the issue commit plus the ones asked for")

	// Every commit that was meant to touch an issue has to have changed the
	// file, not merely rewritten it. Git records no diff for a blob that did
	// not change, so a generator writing the same bytes twice would build a
	// deep history that costs nothing to walk — and the measurement taken on it
	// would be of deduplication rather than of depth.
	changes := r.Git("log", "--format=%H", "--first-parent", "--name-only",
		"--no-renames", gittest.DefaultBranch, "--", "issues")

	var changed int

	for _, line := range strings.Split(changes, "\n") {
		if strings.HasPrefix(line, "issues/") {
			changed++
		}
	}

	require.Equal(t, touching+issues, changed,
		"one change per touching commit, plus the commit that wrote the issues")
}

// The commits that are not issue commits are still commits, and still have to
// be walked to answer a question about history under a pathspec. Trunk in a
// repository using isu is mostly code, so a fixture that made every commit an
// issue commit would measure the wrong half.
func TestGenerateFillsTheRestOfTrunkWithOrdinaryCommits(t *testing.T) {
	r := gittest.Generate(t, gittest.Spec{Issues: 5, Commits: 50, TouchesIssues: 10})

	touching, err := strconv.Atoi(r.Git("rev-list", "--count",
		gittest.DefaultBranch, "--", "issues"))
	require.NoError(t, err)

	require.Equal(t, 11, touching, "ten, plus the commit that wrote the issues")
	require.Equal(t, "50", r.Git("rev-list", "--count", gittest.DefaultBranch))
}

// The working tree is left on the history that was imported. fast-import moves
// the ref without touching the index, so a fixture that did not reset would
// have every later `git add` in the same repository trying to revert it.
func TestGenerateLeavesTheCheckoutOnTheImportedHistory(t *testing.T) {
	r := gittest.Generate(t, gittest.Spec{Issues: 5, Commits: 20, TouchesIssues: 5})

	require.Empty(t, r.Git("status", "--porcelain"),
		"the checkout matches the tip that was imported")
	require.Equal(t, r.Git("rev-parse", gittest.DefaultBranch), r.Head())
}
