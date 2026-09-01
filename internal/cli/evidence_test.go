package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// resolved is a repository holding one open issue at trunk, on a branch that
// says it has resolved it. What the branch did *besides* that is the subject of
// every test below.
func resolved(t *testing.T, opts ...gittest.IssueOption) *gittest.Repo {
	t.Helper()

	open := append([]gittest.IssueOption{
		gittest.Title("Login retries drop the second attempt"),
		gittest.Owner("dmitry"), gittest.Field("repro", "post twice"),
	}, opts...)

	return configured(t).
		Issue("ISU-7f3akq", open...).Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", append(open, gittest.State("resolved"))...).
		Commit("claim ISU-7f3akq")
}

// The rule that makes a claim a claim. `isu claim` writes `state: resolved`
// before any work starts, so a branch that claimed something and did nothing
// looks, in the file, exactly like one that resolved it. What separates them is
// the diff beside it.
func TestABranchCarryingNothingButAClaimFails(t *testing.T) {
	t.Parallel()

	found := only(t, rules(t, resolved(t)))

	require.Equal(t, "evidence", found.Check)
	require.Equal(t, "fail", found.Severity)
	require.Equal(t, "ISU-7f3akq", found.ID)
	require.Contains(t, found.Message, "a claim and not a resolution")
}

func TestAResolutionWithCodeBesideItPasses(t *testing.T) {
	t.Parallel()

	r := resolved(t).File("login.go", "package login\n").Commit("fix the retry")

	require.Empty(t, rules(t, r).Findings)
}

// isu claim, isu resolve and a pull request, in that order and through the
// product rather than through the fixture builder.
func TestTheEvidenceRuleReadsWhatIsuClaimAndIsuResolveActuallyWrite(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.Title("Login retries"), gittest.Owner("dmitry"),
			gittest.Field("repro", "post twice")).
		Commit("report ISU-7f3akq").
		WithRemote()

	isu(t, r.Dir(), "claim", "ISU-7f3akq").ok(t)
	r.Checkout("isu/ISU-7f3akq")

	claimed := only(t, rules(t, r))
	require.Equal(t, "evidence", claimed.Check)

	r.File("login.go", "package login\n").Commit("fix the retry")
	isu(t, r.Dir(), "resolve", "ISU-7f3akq").ok(t)

	require.Empty(t, rules(t, r).Findings,
		"the claim, the work and the resolution together are a pull request")
}

func TestASpikeIsResolvedByTheAnswerBesideTheQuestion(t *testing.T) {
	t.Parallel()

	spike := []gittest.IssueOption{
		gittest.Title("What does contention cost?"), gittest.Type("spike"),
		gittest.Owner("dmitry"), gittest.Field("question", "how much?"),
	}

	r := configured(t).
		Issue("ISU-7f3akq", spike...).Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", append(spike, gittest.State("resolved"))...).
		Commit("resolve ISU-7f3akq")

	found := only(t, rules(t, r))
	require.Equal(t, "evidence", found.Check)
	require.Contains(t, found.Message, "holds nothing but its README")

	// Code outside issues/ is not what a spike owes anybody, so it does not
	// answer this one.
	r.File("login.go", "package login\n").Commit("write some code instead")
	require.Len(t, rules(t, r).Findings, 1)

	r.Issue("ISU-7f3akq", append(spike,
		gittest.State("resolved"),
		gittest.Attachment("decision.md", "we measured it at 3.5ms a ref\n"))...).
		Commit("write the answer down")

	require.Empty(t, rules(t, r).Findings)
}

// A comment is a remark in a thread. What a spike owes the next person is a
// file they can open.
func TestACommentIsNotASpikesAnswer(t *testing.T) {
	t.Parallel()

	spike := []gittest.IssueOption{
		gittest.Title("What does contention cost?"), gittest.Type("spike"),
		gittest.Owner("dmitry"), gittest.Field("question", "how much?"),
	}

	r := configured(t).
		Issue("ISU-7f3akq", spike...).Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", append(spike, gittest.State("resolved"),
			gittest.Comment("2026-08-24-dmitry-01.md", "about 3.5ms\n"))...).
		Commit("resolve ISU-7f3akq")

	require.Contains(t, only(t, rules(t, r)).Message, "holds nothing but its README")
}

// Dropping a duplicate in the same pull request as the fix is a normal thing to
// do. Refusing it teaches people to split one review into two.
func TestADropThatAlsoChangesCodeWarnsAndExitsZero(t *testing.T) {
	t.Parallel()

	bug := []gittest.IssueOption{
		gittest.Title("Login retries"), gittest.Owner("dmitry"),
		gittest.Field("repro", "post twice"),
	}

	r := configured(t).
		Issue("ISU-7f3akq", bug...).Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", append(bug, gittest.State("dropped"),
			gittest.Field("reason", "the same as ISU-40b1cc"),
			gittest.Field("resolution", "duplicate"))...).
		File("login.go", "package login\n").
		Commit("drop the duplicate in the pull request that fixes it")

	got := isu(t, r.Dir(), "check", "--json").ok(t)

	found := only(t, decode[CheckPayload](t, got))
	require.Equal(t, "evidence", found.Check)
	require.Equal(t, "warn", found.Severity)
	require.Contains(t, found.Message, "login.go")
}

func TestADropWithNothingBesideItIsFine(t *testing.T) {
	t.Parallel()

	bug := []gittest.IssueOption{
		gittest.Title("Login retries"), gittest.Owner("dmitry"),
		gittest.Field("repro", "post twice"),
	}

	r := configured(t).
		Issue("ISU-7f3akq", bug...).Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", append(bug, gittest.State("dropped"),
			gittest.Field("reason", "we will not do this"),
			gittest.Field("resolution", "wontfix"))...).
		Commit("drop ISU-7f3akq")

	require.Empty(t, rules(t, r).Findings)
}

// A drop with no resolution cannot tell a duplicate from a won't-fix, and the
// difference is the first thing anyone asks. The schema is what says so —
// `state: dropped` without it does not satisfy Validate — and this asserts that
// the run fails rather than which rule caught it, because two rules reporting
// one line would give a reader two things to fix that are one thing.
func TestADropWithNoResolutionFailsTheRun(t *testing.T) {
	t.Parallel()

	bug := []gittest.IssueOption{
		gittest.Title("Login retries"), gittest.Owner("dmitry"),
		gittest.Field("repro", "post twice"),
	}

	r := configured(t).
		Issue("ISU-7f3akq", bug...).Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", append(bug, gittest.State("dropped"),
			gittest.Field("reason", "we will not do this"))...).
		Commit("drop ISU-7f3akq without saying which kind of drop it is")

	found := only(t, rules(t, r))

	require.Equal(t, "schema", found.Check)
	require.Contains(t, found.Message, "resolution")
	require.False(t, rules(t, r).OK)
}

// An issue this branch never touched is not this branch's to answer for.
func TestAnIssueTheBranchDidNotTouchIsNotItsBusiness(t *testing.T) {
	t.Parallel()

	r := resolved(t)

	r.Checkout(gittest.DefaultBranch).
		Branch("topic").Checkout("topic").
		File("login.go", "package login\n").Commit("unrelated work")

	require.Empty(t, rules(t, r).Findings,
		"the claiming branch is somebody else's proposal, and this run is about "+
			"the one that is checked out")
}

// An import writes thousands of issues that were closed years ago, on a branch
// that changes nothing outside issues/ because there is nothing else to change.
// Nothing anybody was tracking was marked done, so nothing is owed.
func TestAnIssueThatArrivesAlreadyResolvedIsNotAResolution(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Branch("import/jira").Checkout("import/jira").
		Issue("PROJ-1234", gittest.Title("Something Jira closed in 2019"),
			gittest.Owner("dmitry"), gittest.Type("chore"), gittest.State("resolved")).
		Commit("import one closed issue")

	require.Empty(t, rules(t, r).Findings)
}

func TestTheEvidenceRuleIsInTheBranchScope(t *testing.T) {
	t.Parallel()

	r := resolved(t)

	require.Len(t, rules(t, r, "--scope", "branch").Findings, 1)
	require.Empty(t, rules(t, r, "--scope", "tree").Findings)
}
