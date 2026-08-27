package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// M3-S3. A claim is a branch whose issue file says resolved where trunk says
// open. Who made it and when are the first commit on that branch — not the
// branch tip, which moves every time the claimant pushes more work and would
// make a claim that never ages.

func TestAClaimNamesWhoFlippedTheStateAndWhen(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("alice").Backdate(3).
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq")

	flip := r.Head()

	// More work on top, by somebody else, today. This is what moves the branch
	// tip, and none of it may move the claim.
	r.As("bob").Backdate(0).
		File("login.go", "package login\n").Commit("retry the login three times").
		Checkout(gittest.DefaultBranch)

	tip := r.Git("rev-parse", "isu/ISU-7f3akq")
	require.NotEqual(t, flip, tip, "the fixture pushed work on top")

	board := model.Derive(load(t, r, time.Now()))
	claims := item(t, board, "ISU-7f3akq").Claims

	require.Len(t, claims, 1)
	require.Equal(t, "refs/heads/isu/ISU-7f3akq", claims[0].Ref)
	require.Equal(t, flip, claims[0].Commit, "the claim is the commit that flipped the state")
	require.Equal(t, "alice", claims[0].Claimant)
	require.Equal(t, "alice@example.invalid", claims[0].Email)
	require.WithinDuration(t, time.Now().Add(-72*time.Hour), claims[0].When, time.Minute,
		"the claim is three days old, whatever the tip's date says")
	require.InDelta(t, 72*time.Hour, claims[0].Age, float64(time.Minute))
}

func TestAClaimOlderThanStaleDaysIsStale(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("alice").Backdate(11).
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Backdate(0).Checkout(gittest.DefaultBranch)

	board := model.Derive(load(t, r, time.Now()))

	require.Equal(t, model.StatusInProgress, statusOf(t, board, "ISU-7f3akq"))
	require.True(t, item(t, board, "ISU-7f3akq").Claims[0].Stale,
		"eleven days against the default seven")
	require.True(t, item(t, board, "ISU-7f3akq").Stale())
}

func TestAClaimYoungerThanStaleDaysIsNot(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("alice").Backdate(3).
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Backdate(0).Checkout(gittest.DefaultBranch)

	board := model.Derive(load(t, r, time.Now()))

	require.False(t, item(t, board, "ISU-7f3akq").Claims[0].Stale)
	require.False(t, item(t, board, "ISU-7f3akq").Stale())
}

func TestTwoBranchesClaimingOneIssueAreContendedAndBothNamed(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("alice").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch).
		Branch("isu/ISU-7f3akq-2").Checkout("isu/ISU-7f3akq-2").
		As("bob").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	board := model.Derive(load(t, r, time.Now()))
	got := item(t, board, "ISU-7f3akq")

	require.Equal(t, model.StatusInProgress, got.Status)
	require.True(t, got.Contended(), "two people are about to do the same work")
	require.Equal(t, []string{"alice", "bob"}, claimants(got),
		"both are named, in ref order, because the board cannot tell them who is right")
}

func TestABranchWithNoStateFlipHasNoClaimant(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("feature/retries").Checkout("feature/retries").
		As("alice").
		File("login.go", "package login\n").Commit("work on the login flow").
		Checkout(gittest.DefaultBranch)

	board := model.Derive(load(t, r, time.Now()))
	got := item(t, board, "ISU-7f3akq")

	require.Empty(t, got.Claims)
	require.False(t, got.Contended())
	require.False(t, got.Stale())
	require.Equal(t, model.StatusOpen, got.Status)
}

// `isu unclaim` flips the state back and leaves the branch standing: a one-word
// command must not throw away work. What it releases is the claim, and what it
// leaves is an ordinary branch the board says nothing about.
func TestUnclaimingStopsTheIssueReadingAsInProgress(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("alice").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		File("login.go", "package login\n").Commit("some work").
		Issue("ISU-7f3akq").Commit("unclaim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	board := model.Derive(load(t, r, time.Now()))

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-7f3akq"))
	require.Empty(t, item(t, board, "ISU-7f3akq").Claims)
	require.Contains(t, r.Branches(), "isu/ISU-7f3akq", "the work is still there")
}

// A claim is released when the work lands, and there is no sweep: terminal
// trunk state wins the precedence contest, so nothing has to remember to tidy
// a claim up.
func TestAClaimingBranchDeletedAfterTheMergeLeavesTheIssueDone(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("alice").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch).
		Merge("isu/ISU-7f3akq").
		DeleteBranch("isu/ISU-7f3akq")

	board := model.Derive(load(t, r, time.Now()))
	got := item(t, board, "ISU-7f3akq")

	require.Equal(t, model.StatusDone, got.Status)
	require.Empty(t, got.Claims)
	require.False(t, got.Stale(), "a finished issue is not a stale claim for ever after")
}

// PLAN.md M3-S3: contention and staleness come out of what M2 already loaded
// plus one log per claiming branch, and nothing in internal/model runs git.
func TestClaimsCostOneLogPerClaimingBranchAndDerivationCostsNothing(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Issue("ISU-40b1cc").Issue("ISU-39ka2p").Commit("three issues").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch).
		Branch("isu/ISU-40b1cc").Checkout("isu/ISU-40b1cc").
		Issue("ISU-40b1cc", gittest.State("resolved")).Commit("claim ISU-40b1cc").
		Checkout(gittest.DefaultBranch).
		Branch("feature/retries").Checkout("feature/retries").
		File("login.go", "package login\n").Commit("no claim here").
		Checkout(gittest.DefaultBranch)

	loader, err := repo.Open(r.Dir())
	require.NoError(t, err)

	ctx := t.Context()

	loaded, err := loader.LoadBoard(ctx, repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	refs := model.ClaimRefs(loaded)
	require.Equal(t,
		[]string{"refs/heads/isu/ISU-40b1cc", "refs/heads/isu/ISU-7f3akq"}, refs,
		"the branch that flipped nothing is not asked about")

	before := loader.Processes()

	claims, err := loader.LoadFirstCommits(ctx, gittest.DefaultBranch, refs)
	require.NoError(t, err)
	require.Equal(t, int64(len(refs)), loader.Processes()-before,
		"one git log per claiming branch, and none for the branches that claim nothing")

	history, err := loader.LoadHistory(ctx, gittest.DefaultBranch)
	require.NoError(t, err)

	spent := loader.Processes()

	board := model.Derive(model.Input{
		Loaded: loaded, History: history, Claims: claims,
		Config: defaultConfig(), Now: time.Now(),
	})

	require.Equal(t, spent, loader.Processes(),
		"deriving is a pure function over what was loaded, and there are no exceptions")
	require.Equal(t, model.StatusInProgress, statusOf(t, board, "ISU-7f3akq"))
	require.Equal(t, model.StatusInProgress, statusOf(t, board, "ISU-40b1cc"))
	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-39ka2p"))
}

// A claim whose branch could not be logged is still a claim: what the file says
// is the claim, and the first commit only says who made it. Losing the whole
// row because one lookup came back empty would be the board hiding a claim
// because it could not name a person.
func TestAClaimWithNoFirstCommitIsStillAClaim(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	in := load(t, r, time.Now())
	in.Claims = nil

	board := model.Derive(in)
	got := item(t, board, "ISU-7f3akq")

	require.Equal(t, model.StatusInProgress, got.Status)
	require.Len(t, got.Claims, 1)
	require.Empty(t, got.Claims[0].Claimant)
	require.False(t, got.Claims[0].Stale, "an unknown date is not an old one")
}

func TestLoadFirstCommitsIsEmptyWithNothingToLookUp(t *testing.T) {
	r := gittest.New(t).Issue("ISU-7f3akq").Commit("report ISU-7f3akq")

	loader, err := repo.Open(r.Dir())
	require.NoError(t, err)

	before := loader.Processes()

	claims, err := loader.LoadFirstCommits(t.Context(), gittest.DefaultBranch, nil)
	require.NoError(t, err)

	require.Empty(t, claims)
	require.Equal(t, before, loader.Processes(), "nothing to ask about costs nothing")
}

func claimants(i *model.Item) []string {
	out := make([]string, 0, len(i.Claims))
	for _, claim := range i.Claims {
		out = append(out, claim.Claimant)
	}

	return out
}
