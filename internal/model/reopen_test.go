package model_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// M3-S4. `reopened` is the one status that cannot be answered from the current
// content of any ref: the file at trunk says `open`, exactly as it did the day
// the issue was reported, and the difference between the two is a commit that
// is no longer the tip.
//
// Every case here is run twice, once with the claiming branch deleted after the
// merge — which is what a forge does — because a repository with no branches
// left is where an answer that quietly depended on one stops working.

func TestReopenAcrossTheLifecycle(t *testing.T) {
	for _, tidy := range []bool{false, true} {
		name := "branch still there"
		if tidy {
			name = "branch deleted after the merge"
		}

		t.Run(name, func(t *testing.T) {
			t.Run("merged then reverted reads as reopened", func(t *testing.T) {
				r := merged(t, "ISU-7f3akq", tidy)
				revertMerge(t, r, r.Head())

				board := derive(t, r)
				got := item(t, board, "ISU-7f3akq")

				require.True(t, got.Reopened, "trunk resolved it once and says open now")

				if tidy {
					require.Equal(t, model.StatusReopened, got.Status)

					return
				}

				// A branch left standing through a revert still says resolved
				// where trunk now says open, and that is a claim by the only
				// definition of one there is. In progress beats reopened, and
				// the annotation above survives, so the fact is not lost — the
				// board is saying somebody's branch disagrees with trunk,
				// which is exactly the situation.
				require.Equal(t, model.StatusInProgress, got.Status)
				require.Len(t, got.Claims, 1)
			})

			t.Run("merged, reverted and re-fixed reads as done", func(t *testing.T) {
				r := merged(t, "ISU-7f3akq", tidy)
				revertMerge(t, r, r.Head())
				r.Issue("ISU-7f3akq", gittest.State("resolved")).Commit("fix it again")

				board := derive(t, r)

				require.Equal(t, model.StatusDone, statusOf(t, board, "ISU-7f3akq"))
				require.False(t, item(t, board, "ISU-7f3akq").Reopened,
					"it is resolved at trunk now, so there is nothing to annotate")
			})

			t.Run("never resolved reads as open", func(t *testing.T) {
				r := gittest.New(t).
					Issue("ISU-7f3akq").Commit("report ISU-7f3akq")

				if tidy {
					r.Branch("wip").DeleteBranch("wip")
				}

				board := derive(t, r)

				require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-7f3akq"))
				require.False(t, item(t, board, "ISU-7f3akq").Reopened)
			})
		})
	}
}

// Dropping an issue and later reopening it is not a reopen. The table in
// the plan names `resolved`, and the two are not the same event: undropping is a
// triage decision somebody made on purpose, where a reopen is the repository
// reporting that a fix did not hold.
func TestDroppedAndOpenedAgainIsNotAReopen(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Issue("ISU-7f3akq", droppedIssue...).Commit("drop ISU-7f3akq").
		Issue("ISU-7f3akq").Commit("pick ISU-7f3akq back up")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-7f3akq"))
	require.False(t, item(t, board, "ISU-7f3akq").Reopened)
}

// An issue whose folder was deleted at trunk and written again holds a removal
// in the middle of its history, and the removal does not break the chain.
//
// Issues are files, so somebody can remove a folder and commit it, and the id
// can come back afterwards — by reverting that commit, or by re-running an
// import, since imported issues keep their source key verbatim and so repeat
// exactly. Decided in #8, having been asked by this test: an id is permanent
// from creation, so the same id is the same issue and trunk did resolve it
// once. The reading that lost was that a removal ends the file's story, as
// M2-S4 treats a rename; a rename produces a different id and this does not.
func TestAnIssueDeletedAndReportedAgainIsAReopen(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("resolve ISU-7f3akq")

	r.Git("rm", "--quiet", "-r", "--", "issues/ISU-7f3akq")
	r.Commit("delete the folder").
		Issue("ISU-7f3akq").Commit("report it again")

	board := derive(t, r)

	require.Equal(t, model.StatusReopened, statusOf(t, board, "ISU-7f3akq"))
	require.True(t, item(t, board, "ISU-7f3akq").Reopened)
}

// The fold is over the history index and nothing else, so it can be asserted
// without a repository at all.
func TestReopenedIsAFoldOverTheHistoryIndex(t *testing.T) {
	require.False(t, model.Reopened(nil), "an issue trunk has never seen")

	require.True(t, model.Reopened([]repo.StateAt{
		{State: "open"}, {State: "resolved"}, {State: "open"},
	}))
	require.False(t, model.Reopened([]repo.StateAt{
		{State: "open"}, {State: "resolved"},
	}), "the latest entry is not open")
	require.False(t, model.Reopened([]repo.StateAt{
		{State: "open"},
	}), "nothing earlier was resolved")
	require.False(t, model.Reopened([]repo.StateAt{
		{State: "resolved"}, {Removed: true},
	}), "a removal is not an open issue")
	require.True(t, model.Reopened([]repo.StateAt{
		{State: "resolved"}, {Removed: true}, {State: "open"},
	}), "a removal in the middle is not a state, and the rule reads the states")
}

// merged is an issue reported on trunk, claimed on a branch, fixed there and
// merged — optionally with the branch deleted afterwards, which is what a forge
// does. It leaves the merge commit at HEAD.
func merged(t *testing.T, id string, tidy bool) *gittest.Repo {
	t.Helper()

	r := gittest.New(t).
		Issue(id).Commit("report "+id).
		Branch("isu/"+id).Checkout("isu/"+id).
		Issue(id, gittest.State("resolved")).Commit("claim "+id).
		File("login.go", "package login\n").Commit("retry the login three times").
		Checkout(gittest.DefaultBranch).
		Merge("isu/" + id)

	if tidy {
		r.DeleteBranch("isu/" + id)
	}

	return r
}

// revertMerge undoes a merge commit. Reverting one needs the mainline named:
// git cannot work out on its own which side of the merge you want back.
func revertMerge(t *testing.T, r *gittest.Repo, ref string) *gittest.Repo {
	t.Helper()

	r.Git("revert", "--no-edit", "-m", "1", ref)

	return r
}
