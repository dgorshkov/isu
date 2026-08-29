package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// M3-S5. The whole lifecycle, with nothing but squash merges.
//
// Squash is what most teams have their forge set to, and it is the merge
// strategy that breaks a design built on commit metadata: it collapses
// authorship, rewrites dates, and leaves one commit where there were five. It
// does not touch the file. Every derivation in this package reads blob content,
// so the model should not be able to tell the difference — and this test exists
// to find out, not to assume.
func TestTheWholeLifecycleUnderSquashMerges(t *testing.T) {
	const id = "ISU-7f3akq"

	// Report. Somebody who is not on the team opens an issue on a branch.
	r := gittest.New(t).
		File("README.md", "# a project\n").Commit("the project").
		Branch("report/"+id).Checkout("report/"+id).
		As("sam").
		Issue(id, gittest.Type("bug"), gittest.Field("repro", "log in twice")).
		Commit("report " + id).
		Checkout(gittest.DefaultBranch)

	require.Equal(t, model.StatusAwaitingTriage, statusOf(t, derive(t, r), id))

	// Triage. The report is squashed onto trunk and its branch is deleted.
	r.SquashMerge("report/"+id, "triage "+id).DeleteBranch("report/" + id)

	require.Equal(t, model.StatusOpen, statusOf(t, derive(t, r), id))

	// Claim. The state is flipped on isu/<ID> before any work starts.
	r.Branch("isu/"+id).Checkout("isu/"+id).
		As("alice").Backdate(2).
		Issue(id, gittest.Type("bug"), gittest.Field("repro", "log in twice"),
			gittest.State("resolved")).
		Commit("claim " + id)

	flip := r.Head()

	r.Backdate(0).Checkout(gittest.DefaultBranch)

	claimed := item(t, model.Derive(load(t, r, time.Now())), id)
	require.Equal(t, model.StatusInProgress, claimed.Status)
	require.Equal(t, "alice", claimed.Claims[0].Claimant)
	require.Equal(t, flip, claimed.Claims[0].Commit)

	// Fix. Real work lands on the claiming branch, which moves its tip.
	r.Checkout("isu/"+id).
		File("login.go", "package login\n").Commit("retry the login three times").
		Checkout(gittest.DefaultBranch)

	working := item(t, model.Derive(load(t, r, time.Now())), id)
	require.Equal(t, model.StatusInProgress, working.Status)
	require.Equal(t, flip, working.Claims[0].Commit, "the claim did not move with the tip")
	require.Equal(t, claimed.Claims[0].When, working.Claims[0].When)

	// Merge. One squash commit, composed by the forge from the pull request's
	// title — so the subject says nothing about the issue, and the trailer is
	// the only thing that does. The branch goes with it.
	r.SquashMergeWithBody("isu/"+id,
		"Retry the login three times (#42)", model.ResolvesTrailer+": "+id).
		DeleteBranch("isu/" + id)

	board := model.Derive(load(t, r, time.Now()))
	require.Equal(t, model.StatusDone, statusOf(t, board, id))
	require.Empty(t, item(t, board, id).Claims,
		"the claim was released by the work landing, and there is no sweep")
	require.Empty(t, board.Names(), "not one branch is left in the repository")

	// The trunk commit that set resolved names the issue it resolved. This is
	// the one question file content cannot answer, which is why isu resolve
	// writes a trailer.
	resolving := resolvingCommit(t, r, id)
	ids, tier := model.Resolves(gittest.DefaultPrefix,
		r.Git("log", "-1", "--format=%s", resolving),
		r.Git("log", "-1", "--format=%b", resolving))

	require.Equal(t, []string{id}, ids)
	require.Equal(t, model.TierTrailer, tier)

	// Revert. The fix did not hold.
	r.Revert(resolving)

	reverted := item(t, model.Derive(load(t, r, time.Now())), id)
	require.Equal(t, model.StatusReopened, reverted.Status)
	require.True(t, reverted.Reopened)
}

// A squash whose subject carries the id, and a squash whose subject is only a
// pull request title. The second must come back with nothing rather than with a
// guess: a wrong link is worse than a missing one, because nothing downstream
// can tell it from a right one.
func TestResolvesFallsBackToTheSubjectAndStopsThere(t *testing.T) {
	for _, tc := range []struct {
		name    string
		subject string
		want    []string
		tier    model.Tier
	}{
		{
			name:    "the id opens the subject",
			subject: "ISU-7f3akq: retry the login three times",
			want:    []string{"ISU-7f3akq"},
			tier:    model.TierSubject,
		},
		{
			name:    "the id closes it, with a full stop",
			subject: "Retry the login three times, fixing ISU-7f3akq.",
			want:    []string{"ISU-7f3akq"},
			tier:    model.TierSubject,
		},
		{
			name:    "the id is in brackets",
			subject: "Retry the login three times (ISU-7f3akq)",
			want:    []string{"ISU-7f3akq"},
			tier:    model.TierSubject,
		},
		{
			name:    "two ids in one squash",
			subject: "ISU-7f3akq, ISU-40b1cc: the login flow",
			want:    []string{"ISU-7f3akq", "ISU-40b1cc"},
			tier:    model.TierSubject,
		},
		{
			name:    "the same id twice",
			subject: "ISU-7f3akq: retry the login (ISU-7f3akq)",
			want:    []string{"ISU-7f3akq"},
			tier:    model.TierSubject,
		},
		{
			name:    "only a pull request title",
			subject: "Retry the login three times (#42)",
			tier:    model.TierNone,
		},
		{
			name:    "a key from somebody else's tracker",
			subject: "PROJ-1234: retry the login three times",
			tier:    model.TierNone,
		},
		{
			name:    "the prefix with nothing after it",
			subject: "ISU- is not an id",
			tier:    model.TierNone,
		},
		{
			name:    "a word that starts with the prefix",
			subject: "ISUZU-1 is a different repository",
			tier:    model.TierNone,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids, tier := model.Resolves(gittest.DefaultPrefix, tc.subject, "")

			require.Equal(t, tc.want, ids)
			require.Equal(t, tc.tier, tier)
		})
	}
}

func TestResolvesReadsTheTrailerBeforeTheSubject(t *testing.T) {
	ids, tier := model.Resolves(gittest.DefaultPrefix,
		"ISU-40b1cc: something else entirely",
		"Some explanation.\n\nIsu-Resolves: ISU-7f3akq\n")

	require.Equal(t, []string{"ISU-7f3akq"}, ids)
	require.Equal(t, model.TierTrailer, tier,
		"the trailer is what isu wrote, and the subject is what a forge composed")
}

func TestResolvesReadsATrailerNamingSeveralIssues(t *testing.T) {
	ids, tier := model.Resolves(gittest.DefaultPrefix, "a merge queue composed this",
		"Isu-Resolves: ISU-7f3akq, ISU-40b1cc\n")

	require.Equal(t, []string{"ISU-7f3akq", "ISU-40b1cc"}, ids)
	require.Equal(t, model.TierTrailer, tier)
}

// Git matches a trailer's token without regard to case, and so does this: a
// human typing the line by hand should not have to match isu's capitals.
func TestResolvesMatchesTheTrailerTokenWithoutRegardToCase(t *testing.T) {
	ids, tier := model.Resolves(gittest.DefaultPrefix, "no id here",
		"isu-resolves:   ISU-7f3akq  \n")

	require.Equal(t, []string{"ISU-7f3akq"}, ids)
	require.Equal(t, model.TierTrailer, tier)
}

// An imported issue keeps its source key verbatim, so a repository's own prefix
// is the anchor for the subject tier and there is nothing to anchor on without
// one. The trailer needs no anchor: it says what it is.
func TestResolvesWithNoPrefixReadsOnlyTheTrailer(t *testing.T) {
	ids, tier := model.Resolves("", "ISU-7f3akq: retry the login", "")
	require.Empty(t, ids)
	require.Equal(t, model.TierNone, tier)

	ids, tier = model.Resolves("", "no id here", "Isu-Resolves: PROJ-1234\n")
	require.Equal(t, []string{"PROJ-1234"}, ids)
	require.Equal(t, model.TierTrailer, tier)
}

func TestResolvesRefusesATrailerThatDoesNotNameAnID(t *testing.T) {
	ids, tier := model.Resolves(gittest.DefaultPrefix, "no id here",
		"Isu-Resolves: the login one\n")

	require.Empty(t, ids)
	require.Equal(t, model.TierNone, tier)
}

// resolvingCommit is the trunk commit at which the issue's file first said
// resolved, out of the history index M2-S4 builds.
func resolvingCommit(t *testing.T, r *gittest.Repo, id string) string {
	t.Helper()

	loader, err := repo.Open(r.Dir())
	require.NoError(t, err)

	history, err := loader.LoadHistory(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	for _, at := range history[id] {
		if at.State == issue.StateResolved {
			return at.Commit
		}
	}

	t.Fatalf("%s was never resolved at trunk", id)

	return ""
}
