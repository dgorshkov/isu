package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// claiming puts one open issue on trunk and one claiming branch over it: a
// branch saying `state: resolved` where trunk says open, which is what a claim
// is and the only thing that makes an issue read `in progress`.
func claiming(t *testing.T, branch, claimant string, daysAgo int) *gittest.Repo {
	t.Helper()

	fields := func(extra ...gittest.IssueOption) []gittest.IssueOption {
		return append([]gittest.IssueOption{
			gittest.Title("Login retries"), gittest.Owner("dmitry"),
			gittest.Field("repro", "post twice"),
		}, extra...)
	}

	r := configured(t).
		Backdate(daysAgo+1).
		Issue("ISU-7f3akq", fields()...).Commit("report ISU-7f3akq")

	r.Branch(branch).Checkout(branch).
		As(claimant).Backdate(daysAgo).
		Issue("ISU-7f3akq", fields(gittest.State("resolved"))...).
		Commit("claim ISU-7f3akq").
		As("").Backdate(0).
		Checkout(gittest.DefaultBranch)

	return r
}

func TestTwoBranchesClaimingOneIssueWarn(t *testing.T) {
	t.Parallel()

	r := claiming(t, "isu/ISU-7f3akq", "alice", 1)

	r.Branch("isu/ISU-7f3akq-again").Checkout("isu/ISU-7f3akq-again").
		As("bob").
		Issue("ISU-7f3akq", gittest.Title("Login retries"), gittest.Owner("dmitry"),
			gittest.Field("repro", "post twice"), gittest.State("resolved")).
		Commit("claim ISU-7f3akq too").
		As("").Checkout(gittest.DefaultBranch)

	payload := rules(t, r)
	found := only(t, payload)

	require.Equal(t, "claims", found.Check)
	require.Equal(t, "warn", found.Severity)
	require.Equal(t, "ISU-7f3akq", found.ID)
	require.Contains(t, found.Message, "isu/ISU-7f3akq (alice)")
	require.Contains(t, found.Message, "isu/ISU-7f3akq-again (bob)")

	require.True(t, payload.OK, "two people about to do the same work is worth "+
		"saying and is not a reason to refuse a pull request")
	require.Equal(t, 0, isu(t, r.Dir(), "check").code)
}

func TestAClaimOlderThanStaleDaysWarnsWithItsAgeInDays(t *testing.T) {
	t.Parallel()

	r := claiming(t, "isu/ISU-7f3akq", "alice", 30)

	found := only(t, rules(t, r))

	require.Equal(t, "claims", found.Check)
	require.Equal(t, "warn", found.Severity)
	require.Contains(t, found.Message, "claimed 30 days ago on isu/ISU-7f3akq by alice")
	require.Contains(t, found.Message, "stale_days is 7")
}

func TestAFreshClaimIsNotWorthSaying(t *testing.T) {
	t.Parallel()

	require.Empty(t, rules(t, claiming(t, "isu/ISU-7f3akq", "alice", 1)).Findings)
}

func TestStaleDaysIsWhatTheRepositorySaysItIs(t *testing.T) {
	t.Parallel()

	r := claiming(t, "isu/ISU-7f3akq", "alice", 3)
	r.WriteFile(".isu.yml", "prefix: ISU\nstale_days: 2\n")

	found := only(t, rules(t, r))
	require.Contains(t, found.Message, "stale_days is 2")

	r.WriteFile(".isu.yml", "prefix: ISU\nstale_days: 90\n")
	require.Empty(t, rules(t, r).Findings)
}

// Both of these are statements about a ref set, and a ref set is only as
// current as the last fetch. A run that cannot say which refs it read is a run
// two engineers will argue about.
func TestAWarningAboutClaimsSaysWhichRefsItRead(t *testing.T) {
	t.Parallel()

	local := only(t, rules(t, claiming(t, "isu/ISU-7f3akq", "alice", 30)))
	require.Contains(t, local.Message, "local branches only",
		"a repository with no remote is answering about itself")

	r := claiming(t, "isu/ISU-7f3akq", "alice", 30)
	r.Backdate(3).File("something.txt", "to push\n").Commit("something to push").
		WithRemote().Backdate(0)

	stale := only(t, rules(t, r))
	require.Contains(t, stale.Message, "hours old")
	require.Contains(t, stale.Message, "--fetch")
	require.Contains(t, stale.Message, "refs/heads/",
		"a claim somebody else pushed is not in this answer, and the warning "+
			"says so rather than implying it looked everywhere")
}

func TestTheClaimRulesAreAboutRefsAndSoAreInTheTreeScope(t *testing.T) {
	t.Parallel()

	r := claiming(t, "isu/ISU-7f3akq", "alice", 30)

	require.Len(t, rules(t, r, "--scope", "tree").Findings, 1)
	require.Empty(t, rules(t, r, "--scope", "branch").Findings)
}

func TestGoldenCheckWarnings(t *testing.T) {
	t.Parallel()

	r := claiming(t, "isu/ISU-7f3akq", "alice", 30)

	golden(t, "check/warnings.txt", isu(t, r.Dir(), "check").ok(t).stdout)
}
