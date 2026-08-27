package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// M3-S1. One test per status, then the transitions between them.
//
// The statuses are never stored. Every one of them is a statement about what
// the refs say, so every test here scripts refs and asserts what comes out —
// there is no fixture that sets a status directly, because there is nowhere to
// set one.

func TestDoneIsTerminalStateOnTrunk(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("resolve ISU-7f3akq")

	require.Equal(t, model.StatusDone, statusOf(t, derive(t, r), "ISU-7f3akq"))
}

func TestDroppedIsDroppedOnTrunk(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Issue("ISU-7f3akq", droppedIssue...).Commit("drop ISU-7f3akq")

	require.Equal(t, model.StatusDropped, statusOf(t, derive(t, r), "ISU-7f3akq"))
}

func TestAwaitingTriageIsOnABranchAndNotOnTrunk(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc").Commit("report ISU-40b1cc").
		Branch("report/ISU-7f3akq").Checkout("report/ISU-7f3akq").
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	board := derive(t, r)

	require.Equal(t, model.StatusAwaitingTriage, statusOf(t, board, "ISU-7f3akq"))
	require.Equal(t, []string{"refs/heads/report/ISU-7f3akq"},
		item(t, board, "ISU-7f3akq").Elsewhere,
		"an untriaged report is answerable only by naming the branch it is on")
	require.False(t, item(t, board, "ISU-7f3akq").OnTrunk)
}

// The claim itself is the only source of `in progress`: state: resolved on a
// branch where trunk still says open. There is no advisory side-channel to
// reconcile against what the branches say, because there is no second signal.
func TestInProgressIsResolvedOnABranchWhereTrunkIsOpen(t *testing.T) {
	r := claimed(t, "ISU-7f3akq")

	board := derive(t, r)

	require.Equal(t, model.StatusInProgress, statusOf(t, board, "ISU-7f3akq"))
	require.True(t, item(t, board, "ISU-7f3akq").OnTrunk)
}

func TestReopenedIsResolvedAtAnEarlierTrunkCommitAndOpenNow(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("resolve ISU-7f3akq").
		Revert("HEAD")

	board := derive(t, r)

	require.Equal(t, model.StatusReopened, statusOf(t, board, "ISU-7f3akq"))
	require.True(t, item(t, board, "ISU-7f3akq").Reopened)
}

func TestOpenIsOnTrunkWithNobodyClaimingIt(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-7f3akq"))
	require.Empty(t, item(t, board, "ISU-7f3akq").Elsewhere)
	require.False(t, item(t, board, "ISU-7f3akq").Reopened)
}

// The rows of the table overlap, and without a stated precedence a merged issue
// whose claiming branch was never deleted matches both `done` and `in
// progress`. Terminal trunk state beats every claim.
func TestTerminalTrunkStateBeatsAClaimingBranchThatIsStillThere(t *testing.T) {
	r := claimed(t, "ISU-7f3akq").
		Checkout(gittest.DefaultBranch).
		Merge("isu/ISU-7f3akq")

	board := derive(t, r)

	require.Equal(t, model.StatusDone, statusOf(t, board, "ISU-7f3akq"),
		"the work landed, so the branch nobody tidied up does not make it in progress")
	require.Contains(t, r.Branches(), "isu/ISU-7f3akq", "the branch is still standing")
}

// `in progress` beats `reopened`: somebody actively re-fixing an issue needs to
// show as worked, not as merely broken again. The fact is not lost — reopened
// survives as an annotation on whatever status wins.
func TestInProgressBeatsReopenedAndReopenedSurvivesAsAnAnnotation(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("resolve ISU-7f3akq").
		Revert("HEAD").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	board := derive(t, r)

	require.Equal(t, model.StatusInProgress, statusOf(t, board, "ISU-7f3akq"))
	require.True(t, item(t, board, "ISU-7f3akq").Reopened,
		"the status it lost to does not erase the fact that it was reopened")
}

func TestABranchThatDoesNotFlipTheStateIsNotAClaim(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("feature/retries").Checkout("feature/retries").
		File("login.go", "package login\n").Commit("work on the login flow").
		Issue("ISU-7f3akq", gittest.Title("Login retries, restated")).
		Commit("say what the issue is about").
		Checkout(gittest.DefaultBranch)

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-7f3akq"),
		"somebody's branch is not a claim until they say so by claiming")
	require.Empty(t, item(t, board, "ISU-7f3akq").Claims)
	require.Equal(t, []string{"refs/heads/feature/retries"},
		item(t, board, "ISU-7f3akq").Elsewhere,
		"the branch touched the file, which is a different fact from claiming it")
}

func TestABranchIdenticalToTrunkChangesNothing(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-7f3akq"))
	require.Empty(t, item(t, board, "ISU-7f3akq").Elsewhere)
}

func TestAnIssueClaimedOnTwoBranchesIsStillInProgress(t *testing.T) {
	r := claimed(t, "ISU-7f3akq").
		Checkout(gittest.DefaultBranch).
		Branch("isu/ISU-7f3akq-again").Checkout("isu/ISU-7f3akq-again").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	board := derive(t, r)

	require.Equal(t, model.StatusInProgress, statusOf(t, board, "ISU-7f3akq"))
	require.Len(t, item(t, board, "ISU-7f3akq").Claims, 2)
}

func TestABranchDeletedAfterTheMergeLeavesTheIssueDone(t *testing.T) {
	r := claimed(t, "ISU-7f3akq").
		Checkout(gittest.DefaultBranch).
		Merge("isu/ISU-7f3akq").
		DeleteBranch("isu/ISU-7f3akq")

	board := derive(t, r)

	require.Equal(t, model.StatusDone, statusOf(t, board, "ISU-7f3akq"))
	require.Empty(t, board.Names(), "there is nothing beside trunk left to read")
}

func TestABranchResolvingWhatTrunkHasAlreadyResolvedIsStillDone(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("resolve ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		File("login.go", "package login\n").Commit("the fix").
		Checkout(gittest.DefaultBranch)

	require.Equal(t, model.StatusDone, statusOf(t, derive(t, r), "ISU-7f3akq"))
}

// An issue whose folder a branch deleted is still trunk's issue. Deriving from
// the branch's absence would let one branch take an issue off everybody's
// board.
func TestABranchThatDeletedTheIssueDoesNotTakeItOffTheBoard(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("wip").Checkout("wip")

	r.Git("rm", "--quiet", "-r", "--", "issues/ISU-7f3akq")
	r.Commit("delete the folder").Checkout(gittest.DefaultBranch)

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-7f3akq"))
	require.Empty(t, item(t, board, "ISU-7f3akq").Elsewhere,
		"a ref that does not carry the issue is not somewhere it can be read")
}

// Every id trunk or any branch names is on the board, and each one exactly
// once. The transitions above move an issue between statuses; this asserts the
// board itself is the union rather than one ref's view of it.
func TestTheBoardIsTheUnionOfTrunkAndEveryBranch(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Issue("ISU-40b1cc").Commit("two issues").
		Branch("report/ISU-39ka2p").Checkout("report/ISU-39ka2p").
		Issue("ISU-39ka2p").Commit("report ISU-39ka2p").
		Checkout(gittest.DefaultBranch)

	board := derive(t, r)

	require.Equal(t, []string{"ISU-39ka2p", "ISU-40b1cc", "ISU-7f3akq"}, board.IDs())
}

// A repository with no commits has no trunk ref to name, so it is read through
// HEAD — M2's rule, kept: an unborn HEAD is an empty board and every other name
// that does not resolve is a typo worth reporting.
func TestAnEmptyRepositoryDerivesAnEmptyBoard(t *testing.T) {
	loader, err := repo.Open(gittest.New(t).Dir())
	require.NoError(t, err)

	loaded, err := loader.LoadBoard(t.Context(), repo.BoardSpec{})
	require.NoError(t, err)

	board := model.Derive(model.Input{Loaded: loaded, Config: config.Default()})

	require.Empty(t, board.IDs())
	require.Empty(t, board.Items)
	require.Empty(t, board.Names())
}

// The state a non-epic must declare is missing, so no row of the table matches
// on its value. It is still an issue somebody has to see: `isu check` reports
// the file, and the board renders it as the least surprising thing it can.
func TestAnIssueWithNoUsableStateReadsAsOpen(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq", gittest.Field("state", "in-flight")).
		Commit("report ISU-7f3akq")

	require.Equal(t, model.StatusOpen, statusOf(t, derive(t, r), "ISU-7f3akq"))
}

func TestNowDefaultsToTheClock(t *testing.T) {
	r := claimed(t, "ISU-7f3akq")

	in := load(t, r, time.Time{})
	require.True(t, in.Now.IsZero(), "the fixture asked for the default")

	require.Equal(t, model.StatusInProgress,
		statusOf(t, model.Derive(in), "ISU-7f3akq"))
}

func TestStatusTerminalIsTheTwoAnIssueDoesNotComeBackFrom(t *testing.T) {
	require.True(t, model.StatusDone.Terminal())
	require.True(t, model.StatusDropped.Terminal())

	for _, status := range []model.Status{
		model.StatusAwaitingTriage, model.StatusInProgress,
		model.StatusReopened, model.StatusOpen,
	} {
		require.False(t, status.Terminal(), "%s is not terminal", status)
	}
}

// claimed is the fixture most of this package needs: an issue on trunk, and a
// branch that flipped its state to resolved before any work started. It leaves
// the claiming branch checked out, so a test can push more work on top.
func claimed(t *testing.T, id string) *gittest.Repo {
	t.Helper()

	return gittest.New(t).
		Issue(id).Commit("report "+id).
		Branch("isu/"+id).Checkout("isu/"+id).
		Issue(id, gittest.State("resolved")).Commit("claim " + id)
}
