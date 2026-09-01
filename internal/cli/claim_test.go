package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// claimable is a repository with a remote and one open issue, which is the
// smallest thing a claim needs: the push is the compare-and-swap, so a
// repository with nowhere to push cannot claim at all.
func claimable(t *testing.T) *gittest.Repo {
	t.Helper()

	return configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Something to claim")).
		Commit("report ISU-openly").
		WithRemote()
}

func TestClaimBranchesFlipsTheStateAndPushes(t *testing.T) {
	t.Parallel()

	r := claimable(t)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "claim", "ISU-openly").ok(t))

	require.Equal(t, "ISU-openly", written.ID)
	require.Equal(t, "isu/ISU-openly", written.Branch)
	require.True(t, written.Pushed)

	require.Equal(t, "claim ISU-openly",
		r.Git("log", "-1", "--format=%s", "isu/ISU-openly"))
	require.Contains(t, r.Git("show", "isu/ISU-openly:issues/ISU-openly/README.md"),
		"state: resolved")

	// The board reads the flip, which is the only source of `in progress`.
	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))
	require.Equal(t, "in progress", payload.Issue.Status)
	require.Len(t, payload.Issue.Claims, 1)
}

func TestClaimPushesSomethingThatIsNotTheTrunkTip(t *testing.T) {
	t.Parallel()

	r := claimable(t)
	trunk := r.Head()

	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)

	// A push of the bare trunk tip is a no-op fast-forward git accepts from
	// both claimants and reports as success to each. The state flip is the
	// whole reason that cannot happen here, so the test that would have caught
	// the earlier design belongs in this one.
	require.NotEqual(t, trunk, r.Git("rev-parse", "isu/ISU-openly"))
	require.Equal(t, trunk, r.Git("rev-parse", "isu/ISU-openly^"))
}

func TestClaimLeavesTheWorkingTreeAlone(t *testing.T) {
	t.Parallel()

	r := claimable(t)

	// Claiming happens before any work, which means it happens while somebody
	// is in the middle of something.
	r.WriteFile("scratch.txt", "half-finished\n")

	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)

	require.Equal(t, gittest.DefaultBranch, r.Git("rev-parse", "--abbrev-ref", "HEAD"))
	require.Equal(t, "half-finished\n", r.ReadFile("scratch.txt"))
	require.Contains(t, r.ReadFile("issues/ISU-openly/README.md"), "state: open")
}

func TestASecondClaimFromAnotherCloneIsRefusedAndNamesTheHolder(t *testing.T) {
	t.Parallel()

	// The rejection path, deterministically: two clones of one remote, the
	// first claim wins the push, and the second is not a fast-forward.
	r := claimable(t)
	as(r, "alice")

	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)

	second := clone(t, r).Dir()

	got := isu(t, second, "claim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "already claimed")
	require.Contains(t, got.stderr, "alice", "the loser needs the holder's name, not a git error")

	// And nothing was left behind for the loser to clean up.
	require.NotContains(t, branches(t, second), "isu/ISU-openly")
}

func TestClaimingSomethingAlreadyClaimedHereIsRefused(t *testing.T) {
	t.Parallel()

	r := claimable(t)
	as(r, "alice")

	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)

	got := isu(t, r.Dir(), "claim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "alice")
}

func TestClaimingATerminalIssueIsRefused(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-donede", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Already finished"), gittest.State("resolved")).
		Issue("ISU-dropit", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Never happening"), gittest.State("dropped"),
			gittest.Field("reason", "no"), gittest.Field("resolution", "wontfix")).
		Commit("two terminal issues").
		WithRemote()

	for _, id := range []string{"ISU-donede", "ISU-dropit"} {
		got := isu(t, r.Dir(), "claim", id)

		require.Equal(t, 1, got.code)
		require.Contains(t, got.stderr, "already")
	}
}

func TestClaimingAnIssueNobodyHasIsRefused(t *testing.T) {
	t.Parallel()

	r := claimable(t)

	got := isu(t, r.Dir(), "claim", "ISU-nobody")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "no issue ISU-nobody")
}

func TestClaimWithTheRemoteDetachedLeavesNoBranchBehind(t *testing.T) {
	t.Parallel()

	r := claimable(t).DetachRemote()

	got := isu(t, r.Dir(), "claim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.NotContains(t, branches(t, r.Dir()), "isu/ISU-openly",
		"a claim that could not be pushed was not a claim, and must leave nothing behind")
}

func TestClaimWithNoRemoteAtAllSaysWhy(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Nowhere to push it")).
		Commit("report ISU-openly")

	got := isu(t, r.Dir(), "claim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "no remote")
	require.NotContains(t, branches(t, r.Dir()), "isu/ISU-openly")
}

func TestUnclaimFlipsTheStateBackAndKeepsTheBranch(t *testing.T) {
	t.Parallel()

	r := claimable(t)
	before := r.Git("show", gittest.DefaultBranch+":issues/ISU-openly/README.md")

	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)

	// Work pushed on top of the claim, which unclaim must not throw away.
	r.Checkout("isu/ISU-openly").
		File("fix.txt", "a start\n").
		Commit("some work").
		Checkout(gittest.DefaultBranch)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "unclaim", "ISU-openly").ok(t))

	require.Equal(t, "isu/ISU-openly", written.Branch)
	require.True(t, written.Pushed)

	require.Contains(t, branches(t, r.Dir()), "isu/ISU-openly",
		"a one-word command must not throw away work")
	require.Equal(t, "a start", r.Git("show", "isu/ISU-openly:fix.txt"),
		"gittest.Git trims the trailing newline; the work itself is still there")

	require.Equal(t, before, r.Git("show", "isu/ISU-openly:issues/ISU-openly/README.md"),
		"what unclaim releases is the claim, and the file goes back to what trunk says")

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))
	require.Equal(t, "open", payload.Issue.Status)
	require.Empty(t, payload.Issue.Claims)
}

func TestUnclaimFromTheClaimingBranchItself(t *testing.T) {
	t.Parallel()

	r := claimable(t)

	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)
	r.Checkout("isu/ISU-openly")

	isu(t, r.Dir(), "unclaim", "ISU-openly").ok(t)

	// Writing through the working tree rather than moving the ref underneath
	// it: the file on disk is what the branch says.
	require.Contains(t, r.ReadFile("issues/ISU-openly/README.md"), "state: open")
	require.Empty(t, r.Git("status", "--porcelain"))
}

func TestUnclaimingSomethingNobodyClaimedIsRefused(t *testing.T) {
	t.Parallel()

	r := claimable(t)

	got := isu(t, r.Dir(), "unclaim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "no isu/ISU-openly")
}

func TestUnclaimOnABranchThatNeverFlippedTheStateIsRefused(t *testing.T) {
	t.Parallel()

	r := claimable(t)

	// A branch in the claim namespace that did not flip the state is not a
	// claim, and the board deliberately says nothing about it.
	r.Branch("isu/ISU-openly").Checkout("isu/ISU-openly").
		File("work.txt", "started without saying so\n").
		Commit("work, unclaimed").
		Checkout(gittest.DefaultBranch)

	got := isu(t, r.Dir(), "unclaim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "does not claim")
}

func TestClaimHolderAnswersNothingRatherThanGuessing(t *testing.T) {
	t.Parallel()

	// Losing a race is one thing; being unable to say who won is another, and
	// the second must not become a wrong name.
	s := openSession(t, claimable(t).Dir())

	require.Empty(t, s.claimHolder(t.Context(), "isu/ISU-nobody-pushed-this"))

	// And with no remote to ask at all.
	plain := openSession(t, configured(t).Dir())
	require.Empty(t, plain.claimHolder(t.Context(), "isu/ISU-openly"))
}

// clone is a second working copy of a repository's remote, which is what a race
// needs: two people, one origin.
//
// It is built by the harness rather than by `git clone` so that it comes with
// the same scrubbed configuration every fixture has — a second clone whose
// identity depends on the machine the suite runs on is a second clone that
// names a different holder on every laptop.
func clone(t *testing.T, r *gittest.Repo) *gittest.Repo {
	t.Helper()

	second := gittest.New(t)
	second.Git("remote", "add", "origin", r.RemoteDir())
	second.Git("fetch", "--quiet", "origin")
	second.Git("checkout", "--quiet", "-B", gittest.DefaultBranch,
		"origin/"+gittest.DefaultBranch)

	return second
}

// as sets the identity isu's own commits are written under, which is the repo's
// configuration and not the harness's environment: gittest.As names who writes
// the fixture's commits, and a claim is written by the command.
func as(r *gittest.Repo, name string) {
	r.Git("config", "user.name", name)
	r.Git("config", "user.email", name+"@example.invalid")
}

// branches lists a repository's local branches by asking isu's own git binding,
// so that the assertion reads the same refs the commands wrote.
func branches(t *testing.T, dir string) []string {
	t.Helper()

	s := openSession(t, dir)

	refs, err := s.git.ForEachRef(t.Context(), "refs/heads/")
	require.NoError(t, err)

	var names []string
	for _, ref := range refs {
		names = append(names, ref.Name[len("refs/heads/"):])
	}

	return names
}

func TestAClaimCommitIsNeverTheSameCommitTwice(t *testing.T) {
	t.Parallel()

	// The regression test for the hole the stress run found. PLAN.md §1 argued
	// that the push is a compare-and-swap because two claimants write two
	// different commits — "different author, different timestamp, therefore
	// different object ids". Two claimants under one identity in the same
	// second write the *same* commit: same tree, same parent, same author, same
	// second, same message. Git then tells both of them `Everything up-to-date`
	// and exits 0, and a hundred racing clones produced thirty winners.
	//
	// Both claims below are made by one identity, onto one trunk, milliseconds
	// apart — which is the case that used to collide. They must not.
	r := claimable(t)

	first := decode[Write](t, isu(t, r.Dir(), "--json", "claim", "ISU-openly").ok(t))

	// Wipe it everywhere, as though the claim had never been made.
	r.Git("update-ref", "-d", "refs/heads/isu/ISU-openly")
	r.Git("push", "--quiet", "origin", "--delete", "isu/ISU-openly")

	second := decode[Write](t, isu(t, r.Dir(), "--json", "claim", "ISU-openly").ok(t))

	require.NotEqual(t, first.Commit, second.Commit,
		"a claim has to be something nobody else can have committed, and the state "+
			"flip alone is not: two people can write the identical flip")

	require.Contains(t, r.Git("log", "-1", "--format=%b", "isu/ISU-openly"), claimTrailer,
		"the nonce is what makes the commit unique, so it is in the message")
}
