package repo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

func TestLoadBranchReadsWhatEachCommitDidToWhichIssue(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq", gittest.Owner("dmitry"), gittest.Title("Login retries")).
		Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("alice").
		Issue("ISU-7f3akq", gittest.Owner("dmitry"), gittest.Title("Login retries"),
			gittest.State("resolved")).
		Commit("claim ISU-7f3akq").
		File("login.go", "package login\n").Commit("fix the retry").
		Checkout(gittest.DefaultBranch)

	branch, err := open(t, r).LoadBranch(
		t.Context(), gittest.DefaultBranch, "refs/heads/isu/ISU-7f3akq")
	require.NoError(t, err)

	require.Len(t, branch.Commits, 2, "the branch's own commits, oldest first")
	require.Equal(t, "claim ISU-7f3akq", branch.Commits[0].Subject)
	require.Equal(t, "alice", branch.Commits[0].Author.Name)
	require.Equal(t, []string{"issues/ISU-7f3akq/README.md", "login.go"}, branch.Paths)

	edits := branch.Commits[0].Edits
	require.Len(t, edits, 1)
	require.Equal(t, "ISU-7f3akq", edits[0].ID)
	require.False(t, edits[0].Added)
	require.False(t, edits[0].Removed)
	require.Equal(t, issue.StateOpen, edits[0].Before.State,
		"what the commit found there")
	require.Equal(t, issue.StateResolved, edits[0].After.State,
		"and what it left")

	require.Empty(t, branch.Commits[1].Edits, "the second commit touched no issue")

	require.Equal(t, []string{"login.go"}, branch.Outside(repo.IssuesDir),
		"resolving something requires one of these, which is M5-S3's whole rule")
}

func TestABranchFoldsSeveralCommitsIntoOneChangePerIssue(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq", gittest.Owner("dmitry")).Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		Issue("ISU-7f3akq", gittest.Owner("alice")).Commit("hand it to alice").
		Issue("ISU-7f3akq", gittest.Owner("alice"), gittest.State("resolved")).
		Commit("resolve ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	branch, err := open(t, r).LoadBranch(t.Context(), gittest.DefaultBranch, "refs/heads/topic")
	require.NoError(t, err)

	require.Len(t, branch.Issues, 1)

	change, ok := branch.Change("ISU-7f3akq")
	require.True(t, ok)
	require.Equal(t, "dmitry", change.Before.Owner, "what the first commit found")
	require.Equal(t, "alice", change.After.Owner, "what the last one left")
	require.Equal(t, issue.StateResolved, change.After.State)
	require.False(t, change.Added)
	require.False(t, change.Removed)

	_, touched := branch.Change("ISU-nobody")
	require.False(t, touched)
}

// A branch is what it proposes, and trunk moving on is not something it
// proposed. Diffed at the ends rather than from the merge base, trunk's own
// later work reads as this branch deleting it.
func TestABranchIsMeasuredFromWhereItLeftTrunk(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		File("login.go", "package login\n").Commit("fix the retry").
		Checkout(gittest.DefaultBranch).
		File("unrelated.go", "package other\n").Commit("trunk moved on")

	branch, err := open(t, r).LoadBranch(t.Context(), gittest.DefaultBranch, "refs/heads/topic")
	require.NoError(t, err)

	require.Equal(t, []string{"login.go"}, branch.Paths)
	require.Equal(t, r.Git("rev-parse", "HEAD~1"), branch.Base)
}

func TestABranchThatReportsAnIssueTrunkHasNeverSeen(t *testing.T) {
	r := gittest.New(t).
		File("README.md", "a repository\n").Commit("first").
		Branch("report/ISU-7f3akq").Checkout("report/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.Title("Sign-up 500s")).Commit("report ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	branch, err := open(t, r).LoadBranch(
		t.Context(), gittest.DefaultBranch, "refs/heads/report/ISU-7f3akq")
	require.NoError(t, err)

	change, ok := branch.Change("ISU-7f3akq")
	require.True(t, ok)
	require.True(t, change.Added)
	require.Nil(t, change.Before)
	require.Equal(t, "Sign-up 500s", change.After.Title)
}

func TestABranchThatRemovesAnIssueFolder(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic")

	r.Git("rm", "--quiet", "-r", "issues/ISU-7f3akq")
	r.Commit("take it away again")
	r.Checkout(gittest.DefaultBranch)

	branch, err := open(t, r).LoadBranch(t.Context(), gittest.DefaultBranch, "refs/heads/topic")
	require.NoError(t, err)

	change, ok := branch.Change("ISU-7f3akq")
	require.True(t, ok)
	require.True(t, change.Removed)
	require.Nil(t, change.After)
	require.NotNil(t, change.Before)
}

// A half-written issue file is not a load failure. The schema check is what
// reports it, and it can only report it if the loader hands the rest over.
func TestABranchCarryingAnUnreadableIssueLoadsAnyway(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		File("issues/ISU-7f3akq/README.md", "there is no frontmatter here\n").
		Commit("break it").
		Checkout(gittest.DefaultBranch)

	branch, err := open(t, r).LoadBranch(t.Context(), gittest.DefaultBranch, "refs/heads/topic")
	require.NoError(t, err)

	change, ok := branch.Change("ISU-7f3akq")
	require.True(t, ok)
	require.NotNil(t, change.Before)
	require.Nil(t, change.After, "a file nothing can decode is a file nothing read")
}

// One commit touching two issues, which is what a triage sweep or a claim and
// its epic look like. The edits come back in path order so that two runs over
// one branch read the same way.
func TestOneCommitCanEditSeveralIssues(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-bbbbbb").Issue("ISU-aaaaaa").Commit("report two issues").
		Branch("topic").Checkout("topic").
		Issue("ISU-bbbbbb", gittest.Priority("p0")).
		Issue("ISU-aaaaaa", gittest.Priority("p1")).
		Commit("triage both").
		Checkout(gittest.DefaultBranch)

	branch, err := open(t, r).LoadBranch(t.Context(), gittest.DefaultBranch, "refs/heads/topic")
	require.NoError(t, err)

	require.Len(t, branch.Commits, 1)

	ids := make([]string, 0, 2)
	for _, edit := range branch.Commits[0].Edits {
		ids = append(ids, edit.ID)
	}

	require.Equal(t, []string{"ISU-aaaaaa", "ISU-bbbbbb"}, ids)
}

// The same unreadable file on both sides of two commits is read once. It is the
// cache doing its job, and the assertion is that the second lookup answers the
// same way rather than decoding it again.
func TestAnUnreadableFileIsOnlyDecodedOnce(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		File("issues/ISU-7f3akq/README.md", "there is no frontmatter here\n").
		Commit("break it").
		Issue("ISU-7f3akq", gittest.Title("Fixed again")).
		Commit("put it back").
		Checkout(gittest.DefaultBranch)

	branch, err := open(t, r).LoadBranch(t.Context(), gittest.DefaultBranch, "refs/heads/topic")
	require.NoError(t, err)

	require.Nil(t, branch.Commits[0].Edits[0].After, "the commit that broke it")
	require.Nil(t, branch.Commits[1].Edits[0].Before, "and the commit that found it broken")
	require.Equal(t, "Fixed again", branch.Commits[1].Edits[0].After.Title)
}

func TestABranchWithNothingAheadOfTrunkProposesNothing(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic")

	branch, err := open(t, r).LoadBranch(t.Context(), gittest.DefaultBranch, "refs/heads/topic")
	require.NoError(t, err)

	require.Empty(t, branch.Commits)
	require.Empty(t, branch.Paths)
	require.Empty(t, branch.Issues)
	require.Empty(t, branch.Outside(repo.IssuesDir))
}

func TestLoadBranchRefusesARefThatIsNotThere(t *testing.T) {
	r := gittest.New(t).Issue("ISU-7f3akq").Commit("report ISU-7f3akq")

	loader := open(t, r)

	_, err := loader.LoadBranch(t.Context(), gittest.DefaultBranch, "refs/heads/nope")
	require.Error(t, err)

	_, err = loader.LoadBranch(t.Context(), "refs/heads/nope", gittest.DefaultBranch)
	require.Error(t, err)
}
