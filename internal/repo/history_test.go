package repo_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// states is the sequence of state values an issue's file has held, which is
// what most of these tests are about. The commits and dates beside them are
// asserted where they are the point.
func states(t *testing.T, history repo.History, id string) []string {
	t.Helper()

	entries, ok := history[id]
	require.True(t, ok, "%s is not in the history", id)

	out := make([]string, 0, len(entries))
	for _, at := range entries {
		if at.Removed {
			out = append(out, "removed")
			continue
		}

		out = append(out, string(at.State))
	}

	return out
}

// An issue resolved and then reverted has held three states at trunk, not two:
// it was open when it was created, and creation is a state the file held. See
// the note under M2-S4 in PLAN.md.
func TestLoadHistoryRecordsAResolveAndItsRevert(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq")
	added := r.Head()

	r.Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq")
	resolved := r.Head()

	r.Revert(resolved)
	reverted := r.Head()

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.Equal(t, []string{"open", "resolved", "open"}, states(t, history, "AR-7f3akq"))

	entries := history["AR-7f3akq"]
	require.Equal(t, []string{added, resolved, reverted},
		[]string{entries[0].Commit, entries[1].Commit, entries[2].Commit},
		"each entry names the trunk commit the state changed at")

	require.False(t, entries[0].When.IsZero())
	require.False(t, entries[0].When.After(entries[2].When), "oldest first")
	require.Equal(t, issue.StateResolved, entries[1].State)

	require.False(t, entries[0].Terminal())
	require.True(t, entries[1].Terminal(),
		"M3-S4 asks whether an earlier trunk commit was terminal, and this is the question")
	require.False(t, entries[2].Terminal())
}

// issues/README.md is a file explaining the directory, and a folder nobody can
// name is not an issue. Neither belongs in a history keyed by id.
func TestLoadHistoryIgnoresWhatIsNotAnIssue(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").
		File("issues/README.md", "# how this directory works\n").
		File("issues/not an id/README.md", "---\nschema: 1\n---\n").
		Commit("an issue and some decoys").
		File("issues/README.md", "# rewritten\n").
		Commit("edit the directory's own readme")

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.Equal(t, []string{"AR-7f3akq"}, keys(history))
}

func keys(history repo.History) []string {
	out := make([]string, 0, len(history))
	for id := range history {
		out = append(out, id)
	}
	sort.Strings(out)

	return out
}

func TestLoadHistoryOnAnIssueThatNeverChangedIsOneEntry(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Issue("AR-40b1cc").Commit("add AR-40b1cc").
		File("cmd/fix.go", "package cmd\n").Commit("unrelated work")

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.Equal(t, []string{"open"}, states(t, history, "AR-7f3akq"))
	require.Equal(t, []string{"open"}, states(t, history, "AR-40b1cc"))
}

// Rewriting an issue without changing its state is not a state change. Without
// this, every typo fix in a title would land in the sequence and `reopened`
// would be answered by scanning noise.
func TestLoadHistoryCollapsesACommitThatDidNotChangeTheState(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Issue("AR-7f3akq", gittest.Title("A better title")).Commit("retitle AR-7f3akq").
		Issue("AR-7f3akq", gittest.Title("A better title"), gittest.State("resolved")).
		Commit("resolve AR-7f3akq")

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, []string{"open", "resolved"}, states(t, history, "AR-7f3akq"))
}

// PLAN.md §Squash-merge safety: post-merge questions are answered from file
// content at trunk commits, never from commit metadata. Squash collapses
// authorship; it does not touch the file.
func TestLoadHistoryReadsTheSameSequenceFromASquashAndFromAMerge(t *testing.T) {
	build := func(t *testing.T, squash bool) []string {
		t.Helper()

		r := gittest.New(t).
			Issue("AR-7f3akq").Commit("add AR-7f3akq").
			Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
			Issue("AR-7f3akq", gittest.Title("Halfway there")).Commit("take some notes").
			Issue("AR-7f3akq", gittest.Title("Halfway there"), gittest.State("resolved")).
			File("cmd/fix.go", "package cmd\n").
			Commit("resolve AR-7f3akq").
			Checkout(gittest.DefaultBranch)

		if squash {
			r.SquashMerge("isu/AR-7f3akq", "Resolve the login bug (#42)")
		} else {
			r.Merge("isu/AR-7f3akq")
		}

		history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
		require.NoError(t, err)

		return states(t, history, "AR-7f3akq")
	}

	merged := build(t, false)
	squashed := build(t, true)

	require.Equal(t, []string{"open", "resolved"}, merged,
		"the branch's intermediate commits are not trunk's timeline")
	require.Equal(t, merged, squashed)
}

// A merge is where trunk changed, so it is the commit the change is dated from.
// Without --first-parent git reports it at the branch commit, which was never
// on trunk and carries the branch's own date.
func TestLoadHistoryDatesAChangeFromTheCommitThatLandedIt(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
		Backdate(30).
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq")
	onBranch := r.Head()

	r.Backdate(0).Checkout(gittest.DefaultBranch).Merge("isu/AR-7f3akq")
	merge := r.Head()

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	entries := history["AR-7f3akq"]
	require.Len(t, entries, 2)
	require.Equal(t, merge, entries[1].Commit)
	require.NotEqual(t, onBranch, entries[1].Commit)
}

// Renames are not followed: a moved issue folder is one issue ending and
// another beginning. Following them would mean asking git to guess which of two
// issues a file became, and a wrong guess silently rewrites somebody's history.
func TestLoadHistoryDoesNotFollowARename(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")
	r.Git("mv", "issues/AR-7f3akq", "issues/AR-40b1cc")
	r.Commit("move the folder")

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.Equal(t, []string{"open", "removed"}, states(t, history, "AR-7f3akq"))
	require.Equal(t, []string{"open"}, states(t, history, "AR-40b1cc"))
}

func TestLoadHistoryRecordsADeletedIssue(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq")
	r.Git("rm", "-r", "--quiet", "issues/AR-7f3akq")
	r.Commit("delete AR-7f3akq")

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	entries := history["AR-7f3akq"]
	require.Len(t, entries, 2)
	require.True(t, entries[1].Removed)
	require.Empty(t, entries[1].State)
}

// An epic declares no state at all, so its sequence says when it appeared and
// nothing else. That is not the same fact as a deletion, and the two must not
// read alike.
func TestLoadHistoryOnAnEpic(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-40b1cc", gittest.Type("epic"), gittest.Without("state")).
		Commit("add the epic")

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	entries := history["AR-40b1cc"]
	require.Len(t, entries, 1)
	require.Empty(t, entries[0].State)
	require.False(t, entries[0].Removed)
}

// An issue that only exists on a branch is `awaiting triage`, and trunk's
// history has nothing to say about it.
func TestLoadHistoryIgnoresWhatIsOnlyOnABranch(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("report/AR-40b1cc").Checkout("report/AR-40b1cc").
		Issue("AR-40b1cc").Commit("report AR-40b1cc").
		Checkout(gittest.DefaultBranch)

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.Contains(t, history, "AR-7f3akq")
	require.NotContains(t, history, "AR-40b1cc")
}

func TestLoadHistoryOnAnEmptyRepository(t *testing.T) {
	history, err := open(t, gittest.New(t)).LoadHistory(t.Context(), "HEAD")
	require.NoError(t, err)
	require.Empty(t, history)
}

func TestLoadHistoryRefusesARefThatIsNotThere(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).LoadHistory(t.Context(), "refs/heads/nope")
	require.ErrorContains(t, err, "nope")
}

// A blob that does not parse contributes no state, and does not stop the walk:
// somebody's broken commit in the middle of last year is not a reason for the
// board to refuse to render today.
func TestLoadHistorySkipsABlobItCannotParse(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		WriteFile("issues/AR-7f3akq/README.md", "this was never an issue\n")
	r.Git("add", "--", "issues/AR-7f3akq/README.md")
	r.Commit("break AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq")

	history, err := open(t, r).LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, []string{"open", "resolved"}, states(t, history, "AR-7f3akq"))
}

// The whole reason the walk lives in the git layer rather than in derivation:
// answering `reopened` per issue would be a git process per issue, and M3-S1's
// "no git calls inside" rule would be a lie the moment M3-S4 landed.
func TestLoadHistorySpawnsTwoProcessesWhateverTheIssueCount(t *testing.T) {
	for _, issues := range []int{1, 50, 400} {
		r := gittest.Generate(t, gittest.Spec{Issues: issues})
		loader := open(t, r)

		history, err := loader.LoadHistory(t.Context(), gittest.DefaultBranch)
		require.NoError(t, err)
		require.Len(t, history, issues)
		require.Equal(t, int64(2), loader.Processes(), "%d issues", issues)
	}
}
