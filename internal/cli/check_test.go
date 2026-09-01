package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/check"
	"github.com/dgorshkov/isu/internal/gittest"
)

// The board fixture is a repository with nothing wrong with it, which is the
// case a check suite has to get right first: a suite that reports something
// about a healthy repository is a suite people learn to run with `| grep -v`.

func TestCheckOnARepositoryWithNothingWrongWithItSaysSoAndExitsZero(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := isu(t, r.Dir(), "check").ok(t)

	require.Contains(t, got.stdout, "nothing to report")
	require.Empty(t, got.stderr)
}

func TestCheckInJSONIsTheDocumentedShape(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := decode[CheckPayload](t, isu(t, r.Dir(), "check", "--json").ok(t))

	require.Equal(t, gittest.DefaultBranch, got.Trunk)
	require.True(t, got.OK)
	require.Zero(t, got.Failures)
	require.NotNil(t, got.Findings, "an empty list is a list")
	require.Equal(t, check.Checks.Len(), len(got.Checks),
		"a run with no --scope runs every registered check")
}

func TestCheckNamesTheBranchItIsAbout(t *testing.T) {
	t.Parallel()

	r := board(t).
		Branch("isu/ISU-openly").Checkout("isu/ISU-openly").
		File("cmd/login.go", "package cmd\n").Commit("fix the retry")

	got := decode[CheckPayload](t, isu(t, r.Dir(), "check", "--json").ok(t))

	require.Equal(t, "isu/ISU-openly", got.Head)

	text := isu(t, r.Dir(), "check").ok(t)
	require.Contains(t, text.stdout, "main ← isu/ISU-openly")
}

func TestCheckOnTrunkHasNoBranchToBeAbout(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := decode[CheckPayload](t, isu(t, r.Dir(), "check", "--json").ok(t))

	require.Empty(t, got.Head,
		"a branch that is trunk proposes nothing, and the branch rules have "+
			"nothing to be about")
}

// A pipeline checks out a merge commit and no branch at all, which is the one
// case where the ref under review is not in refs/heads/ — and the case where a
// check suite that could not see it would pass every pull request that added a
// broken issue.
func TestCheckReadsADetachedHeadAndTheIssuesOnlyItCarries(t *testing.T) {
	t.Parallel()

	r := board(t).
		Branch("report/ISU-detach").Checkout("report/ISU-detach").
		Issue("ISU-detach", gittest.Title("Found in a pipeline"), gittest.Type("chore"),
			gittest.Owner("dmitry")).
		Commit("report ISU-detach")

	at := r.Head()
	r.Checkout(gittest.DefaultBranch).Git("checkout", "--quiet", "--detach", at)

	got := decode[CheckPayload](t, isu(t, r.Dir(), "check", "--json").ok(t))

	require.Equal(t, "HEAD", got.Head)

	shown := decode[ShowPayload](t, isu(t, r.Dir(), "show", "ISU-detach", "--json").ok(t))
	require.Equal(t, "ISU-detach", shown.Issue.ID,
		"the detached ref is loaded, so the issues only it carries are on the board")
}

func TestCheckScopePartitionsTheChecks(t *testing.T) {
	t.Parallel()

	r := board(t)

	all := decode[CheckPayload](t, isu(t, r.Dir(), "check", "--json").ok(t))
	tree := decode[CheckPayload](t, isu(t, r.Dir(), "check", "--scope", "tree", "--json").ok(t))
	branch := decode[CheckPayload](t,
		isu(t, r.Dir(), "check", "--scope", "branch", "--json").ok(t))

	require.Equal(t, len(all.Checks), len(tree.Checks)+len(branch.Checks),
		"every check is in exactly one scope")
	require.Subset(t, all.Checks, tree.Checks)
	require.Subset(t, all.Checks, branch.Checks)
}

func TestAnUnknownScopeIsAUsageError(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := isu(t, r.Dir(), "check", "--scope", "everything")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "is not a scope")
}

func TestCheckOnARepositoryWithNoCommitsIsNotAFailure(t *testing.T) {
	t.Parallel()

	// Nothing is committed, so every ref is unborn — including the one isu
	// would compare against. There is nothing wrong with such a repository and
	// there is nothing to say about it.
	r := gittest.New(t).WriteFile(".isu.yml", "prefix: ISU\n")

	got := isu(t, r.Dir(), "check", "--json").ok(t)

	require.Empty(t, decode[CheckPayload](t, got).Head)
}

func TestGoldenCheck(t *testing.T) {
	t.Parallel()

	r := board(t)

	golden(t, "check/clean.txt", isu(t, r.Dir(), "check").ok(t).stdout)
}

func TestTheHelpListsTheChecksThemselves(t *testing.T) {
	t.Parallel()

	help := checkLong()

	for _, c := range check.Checks.All() {
		require.Contains(t, help, c.Name())
		require.Contains(t, help, c.Describe(),
			"a check nobody documented is impossible: the registry line is the help")
	}

	require.True(t, strings.HasPrefix(help, "check runs isu's rules"))
}
