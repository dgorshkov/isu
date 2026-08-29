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

// Zero means none, not all.
//
// It meant all until a review pointed out where that leads: a caller deriving
// the count from the depth — `commits / 5`, which is what the benchmarks do —
// gets zero at small depths, and a fixture that read that as "every commit
// touches an issue" would silently become the one thing the surrounding
// comments warn against four times over. Asking for more than there are commits
// fails the test rather than being quietly rounded down, which is how every
// other method in this harness treats input it cannot honour.
func TestGenerateTreatsNoTouchingCommitsAsNone(t *testing.T) {
	r := gittest.Generate(t, gittest.Spec{Issues: 5, Commits: 30})

	require.Equal(t, "30", r.Git("rev-list", "--count", gittest.DefaultBranch))
	require.Equal(t, "1", r.Git("rev-list", "--count", gittest.DefaultBranch, "--", "issues"),
		"only the commit that wrote the issues touches them")
}

// Every commit in a generated fixture carries one identity, whichever way it
// was written.
//
// Both fast-import paths spelled the tester out by hand until a review found
// it, where the rest of the harness resolves it through one accessor — so a
// fixture's imported commits and its committed ones were free to drift apart,
// silently, because nothing compared them. They are the same identity here or
// this fails, which is the assertion that keeps them wired to one source.
func TestGenerateAttributesEveryCommitTheSameWay(t *testing.T) {
	r := gittest.Generate(t, gittest.Spec{
		Issues: 10, Branches: 2, Commits: 10, TouchesIssues: 3,
	})

	seen := map[string]bool{}
	for _, who := range strings.Split(r.Git("log", "--all", "--format=%cn <%ce>"), "\n") {
		seen[who] = true
	}

	require.Len(t, seen, 1, "one identity across committed and imported commits alike")
	require.True(t, seen["isu tester <tester@example.invalid>"],
		"and it is the harness's own, which is what As overrides")
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
