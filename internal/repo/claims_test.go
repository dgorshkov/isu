package repo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// The claim is the first commit on the branch, and PLAN.md's spelling of that
// lookup returns the last one.
//
// `git log <trunk>..<branch> --reverse --max-count=1` is in section 1 and in
// M3-S3, and it does not answer the question either of them asks: git applies
// the limit during the walk, which starts at the tip, and reverses what
// survived it. One commit reversed is that same commit — the tip, which
// PLAN.md's own Claims section calls the wrong answer, because it moves every
// time the claimant pushes and a claim that moves never ages.
//
// Measured, on the fixture below, before this was written:
//
//	--reverse --max-count=1 -> "more work on top"
//	--reverse               -> "claim ISU-7f3akq", "more work on top"
//
// So the range is walked and the first record taken. That costs the branch's
// own commits rather than a constant, which is affordable — a claiming branch
// is a few commits — and correct, which the pair is not at any price.
func TestLoadFirstCommitsTakesTheFirstCommitAndNotTheTip(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("alice").Backdate(3).
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq")

	flip := r.Head()

	r.As("bob").Backdate(0).
		File("login.go", "package login\n").Commit("more work on top").
		Checkout(gittest.DefaultBranch)

	loader := open(t, r)

	first, err := loader.LoadFirstCommits(
		t.Context(), gittest.DefaultBranch, []string{"refs/heads/isu/ISU-7f3akq"})
	require.NoError(t, err)

	got := first["refs/heads/isu/ISU-7f3akq"]

	require.Equal(t, "refs/heads/isu/ISU-7f3akq", got.Ref)
	require.Equal(t, flip, got.OID)
	require.Equal(t, "claim ISU-7f3akq", got.Subject)
	require.Equal(t, "alice", got.Author.Name)
	require.Equal(t, "alice@example.invalid", got.Author.Email)
	require.WithinDuration(t, time.Now().Add(-72*time.Hour), got.Author.When, time.Minute)

	// The assertion the pair in PLAN.md fails.
	require.NotEqual(t, r.Git("rev-parse", "isu/ISU-7f3akq"), got.OID)
}

// The author date is the claim time and the committer date is not. A rebase
// rewrites the second and leaves the first alone, and a claim that aged
// whenever somebody rebased would be a claim nobody could trust.
func TestLoadFirstCommitsReadsTheAuthorDate(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Backdate(9).
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Backdate(0).Checkout(gittest.DefaultBranch)

	first, err := open(t, r).LoadFirstCommits(
		t.Context(), gittest.DefaultBranch, []string{"refs/heads/isu/ISU-7f3akq"})
	require.NoError(t, err)

	require.WithinDuration(t, time.Now().Add(-9*24*time.Hour),
		first["refs/heads/isu/ISU-7f3akq"].Author.When, time.Minute)
}

// A branch with nothing ahead of trunk has no first commit. It cannot be a
// claim either — a claim is a file that says something trunk's does not — so
// this is a lookup with no answer rather than a failure.
func TestLoadFirstCommitsSkipsARefThatIsNotAheadOfTrunk(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq")

	first, err := open(t, r).LoadFirstCommits(
		t.Context(), gittest.DefaultBranch, []string{"refs/heads/isu/ISU-7f3akq"})
	require.NoError(t, err)

	require.Empty(t, first)
}

func TestLoadFirstCommitsOneProcessPerRef(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("one").Checkout("one").File("a", "a\n").Commit("a").
		Checkout(gittest.DefaultBranch).
		Branch("two").Checkout("two").File("b", "b\n").Commit("b").
		Checkout(gittest.DefaultBranch)

	loader := open(t, r)
	before := loader.Processes()

	first, err := loader.LoadFirstCommits(t.Context(), gittest.DefaultBranch,
		[]string{"refs/heads/one", "refs/heads/two"})
	require.NoError(t, err)

	require.Len(t, first, 2)
	require.Equal(t, int64(2), loader.Processes()-before)
}

func TestLoadFirstCommitsReportsARefThatDoesNotResolve(t *testing.T) {
	r := gittest.New(t).Issue("ISU-7f3akq").Commit("report ISU-7f3akq")

	_, err := open(t, r).LoadFirstCommits(
		t.Context(), gittest.DefaultBranch, []string{"refs/heads/nowhere"})

	require.ErrorContains(t, err, "nowhere")
}
