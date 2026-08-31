package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// The annotations, which are the facts a status alone cannot carry.
//
// PLAN.md §1 makes each of them survive losing the precedence contest — an
// issue that is being re-fixed reads `in progress` and is still `reopened` —
// so each has to be rendered beside whatever status won, and the only way to
// know they are is to build a repository holding each one.

// annotated is a repository where every annotation is true of something.
//
//	ISU-conten  two branches claiming it, by two people
//	ISU-stalec  claimed eleven days ago and gone quiet
//	ISU-revert  resolved, merged, reverted, branch still standing
//	ISU-ghostc  claimed by a branch that is behind trunk, so nobody is named
//	ISU-cyclic  an epic that is its own parent
//	ISU-oddity  a priority the schema does not know, and two blockers
func annotated(t *testing.T) *gittest.Repo {
	t.Helper()

	r := configured(t).
		Backdate(40).
		Issue("ISU-conten", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Two people want this")).
		Issue("ISU-stalec", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Somebody said they were on it")).
		Issue("ISU-revert", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("The fix did not hold")).
		Issue("ISU-ghostc", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Claimed from behind")).
		Issue("ISU-cyclic", gittest.Type("epic"), gittest.Without("state"),
			gittest.Owner("dmitry"), gittest.Title("Its own parent"),
			gittest.Parent("ISU-cyclic")).
		Issue("ISU-oddity", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Waits on two things"), gittest.Parent("ISU-cyclic"),
			gittest.Field("priority", "urgent"),
			gittest.BlockedBy("ISU-conten", "ISU-stalec")).
		Commit("six issues")

	// Contended: two branches, two claimants, one issue.
	for _, who := range []string{"alice", "bob"} {
		r.Backdate(2).As(who).
			Branch(who+"/ISU-conten").Checkout(who+"/ISU-conten").
			Issue("ISU-conten", gittest.Owner("dmitry"), gittest.Type("chore"),
				gittest.Title("Two people want this"), gittest.State("resolved")).
			Commit("claim ISU-conten as " + who).
			Checkout(gittest.DefaultBranch)
	}

	// Stale: a claim eleven days old, against a stale_days of seven.
	r.Backdate(11).As("alice").
		Branch("isu/ISU-stalec").Checkout("isu/ISU-stalec").
		Issue("ISU-stalec", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Somebody said they were on it"), gittest.State("resolved")).
		Commit("claim ISU-stalec").
		Checkout(gittest.DefaultBranch).As("").Backdate(0)

	// Reopened *and* in progress: resolved at trunk, reverted at trunk, and the
	// branch that resolved it left standing. M3-S4 records that these two do
	// not read the same with the branch gone and without it — with it there,
	// the branch still says resolved where trunk now says open, which is a
	// claim by the only definition there is.
	r.Branch("isu/ISU-revert").Checkout("isu/ISU-revert").
		Issue("ISU-revert", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("The fix did not hold"), gittest.State("resolved")).
		Commit("resolve ISU-revert").
		Checkout(gittest.DefaultBranch).
		SquashMerge("isu/ISU-revert", "resolve ISU-revert")

	// A squash rather than a merge commit, because a merge cannot be reverted
	// without saying which side to keep — and because the whole point of
	// PLAN.md's squash-merge safety is that the two read the same.
	r.Revert(r.Head())

	// A claim with nobody named: the branch says resolved where trunk says
	// open, and there is nothing ahead of trunk on it to look an author up in.
	// PLAN.md M3-S3 is explicit that this is still a claim — the file is the
	// claim, and the lookup only names who made it.
	r.Branch("isu/ISU-ghostc").Checkout("isu/ISU-ghostc").
		Issue("ISU-ghostc", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Claimed from behind"), gittest.State("resolved")).
		Commit("resolve ISU-ghostc").
		Checkout(gittest.DefaultBranch).
		Merge("isu/ISU-ghostc")

	r.Issue("ISU-ghostc", gittest.Owner("dmitry"), gittest.Type("chore"),
		gittest.Title("Claimed from behind"), gittest.State("open")).
		Commit("undo ISU-ghostc at trunk, leaving the branch behind trunk")

	return r
}

func TestTheBoardRendersEveryAnnotation(t *testing.T) {
	t.Parallel()

	r := annotated(t)

	got := isu(t, r.Dir(), "board").ok(t)

	golden(t, "board/annotations.txt", got.stdout)

	for _, want := range []string{"contended", "stale", "reopened", "parent cycle", "someone"} {
		require.Containsf(t, got.stdout, want, "the board does not say %q anywhere", want)
	}
}

func TestTheAnnotationsAreWhatTheJSONSays(t *testing.T) {
	t.Parallel()

	r := annotated(t)

	byID := map[string]Issue{}
	for _, group := range decode[BoardPayload](t,
		isu(t, r.Dir(), "--json", "board").ok(t)).Groups {
		for _, item := range group.Issues {
			byID[item.ID] = item
		}
	}

	contended := byID["ISU-conten"]
	require.True(t, contended.Contended)
	require.Len(t, contended.Claims, 2)
	require.Len(t, contended.Elsewhere, 2)

	stale := byID["ISU-stalec"]
	require.True(t, stale.Stale)
	require.True(t, stale.Claims[0].Stale)

	reverted := byID["ISU-revert"]
	require.Equal(t, "in progress", reverted.Status,
		"in progress beats reopened, because somebody re-fixing this needs to "+
			"show as worked rather than as merely broken again")
	require.True(t, reverted.Reopened, "and the fact survives as an annotation")

	ghost := byID["ISU-ghostc"]
	require.Len(t, ghost.Claims, 1)
	require.Empty(t, ghost.Claims[0].Claimant,
		"a claim whose first commit was not looked up is still a claim")

	cyclic := byID["ISU-cyclic"]
	require.NotNil(t, cyclic.Epic)
	require.True(t, cyclic.Epic.Cycle)
}

func TestShowRendersEveryAnnotationToo(t *testing.T) {
	t.Parallel()

	r := annotated(t)

	golden(t, "show/contended.txt", isu(t, r.Dir(), "show", "ISU-conten").ok(t).stdout)
	golden(t, "show/stale.txt", isu(t, r.Dir(), "show", "ISU-stalec").ok(t).stdout)
	golden(t, "show/reverted.txt", isu(t, r.Dir(), "show", "ISU-revert").ok(t).stdout)
	golden(t, "show/blockers.txt", isu(t, r.Dir(), "show", "ISU-oddity").ok(t).stdout)

	claimed := isu(t, r.Dir(), "show", "ISU-ghostc").ok(t)
	require.Contains(t, claimed.stdout, "claimed by someone",
		"the claim is real even when the claimant is not known")

	contended := isu(t, r.Dir(), "show", "ISU-conten").ok(t)
	require.Contains(t, contended.stdout, "contended")

	stale := isu(t, r.Dir(), "show", "ISU-stalec").ok(t)
	require.Contains(t, stale.stdout, "stale")

	reverted := isu(t, r.Dir(), "show", "ISU-revert").ok(t)
	require.Contains(t, reverted.stdout, "in progress (reopened)")
}

func TestAPriorityTheSchemaDoesNotKnowSortsLastAndIsRendered(t *testing.T) {
	t.Parallel()

	r := annotated(t)

	got := isu(t, r.Dir(), "--json", "show", "ISU-oddity").ok(t)
	payload := decode[ShowPayload](t, got)

	require.Equal(t, "urgent", payload.Issue.Priority,
		"a priority isu does not know is reported rather than corrected: what to "+
			"do about it is isu check's call")
	require.Len(t, payload.Blockers, 2)
}

func TestABoardWithNoBranchesHasAnEmptyRefList(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Nothing beside it")).
		Commit("one issue and no branches")

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	require.Empty(t, payload.Refs)
	require.NotNil(t, payload.Refs, "an empty list, not a null one: this is a contract")
}

func TestTwoIssuesThatCannotBeDecodedStillSortAndRender(t *testing.T) {
	t.Parallel()

	// Two, so that the comparator runs: an issue with no readable file has no
	// priority and no created date, and sorting must not reach through the
	// hole for either.
	r := configured(t).
		File("issues/ISU-brokeA/README.md", "not an issue\n").
		File("issues/ISU-brokeB/README.md", "also not an issue\n").
		Commit("two half-written issues")

	got := isu(t, r.Dir(), "board").ok(t)

	require.Contains(t, got.stdout, "ISU-brokeA")
	require.Contains(t, got.stdout, "ISU-brokeB")
	require.Empty(t, readyIDs(t, r.Dir()))
}
