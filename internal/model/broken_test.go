package model_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/model"
)

// An issue whose file at trunk will not decode.
//
// The loader keeps these deliberately — a half-written issue must not blind the
// whole board — and derivation used to read only Set.Issues, so the issue
// vanished from the derived board entirely. Nothing said so, and there was
// nowhere for anything to say so.

func TestAnIssueWhoseTrunkFileIsUnreadableIsStillOnTheBoard(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Issue("ISU-40b1cc").Commit("two issues").
		File("issues/ISU-7f3akq/README.md", "just some prose, and no frontmatter\n").
		Commit("break one of them")

	board := derive(t, r)

	require.Equal(t, []string{"ISU-40b1cc", "ISU-7f3akq"}, board.IDs(),
		"the broken one is still an issue this repository has")

	got := item(t, board, "ISU-7f3akq")

	require.True(t, got.OnTrunk, "trunk carries the folder, whatever the file says")
	require.NotNil(t, got.Broken, "and the board says why it could not be read")
	require.Equal(t, "issues/ISU-7f3akq/README.md", got.Broken.Path)
	require.Equal(t, model.StatusOpen, got.Status,
		"open is what the table says for a state nobody can see")

	require.Contains(t, board.Broken, "ISU-7f3akq",
		"indexed too, for a caller reporting what is wrong rather than walking to find it")
}

// The case that made the old behaviour a lie rather than an omission: a branch
// carrying a readable copy of an issue trunk cannot read.
//
// That used to derive as `awaiting triage` — a report trunk has never seen —
// for an issue trunk has carried since it was filed. It now reads as what it
// is: on trunk, and unreadable there.
func TestABranchCopyDoesNotTurnABrokenTrunkIssueIntoAReport(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		File("issues/ISU-7f3akq/README.md", "just some prose, and no frontmatter\n").
		Commit("break it").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("repair it and claim it").
		Checkout(gittest.DefaultBranch)

	got := item(t, derive(t, r), "ISU-7f3akq")

	require.True(t, got.OnTrunk)
	require.NotEqual(t, model.StatusAwaitingTriage, got.Status,
		"trunk has carried this issue all along")
	require.Equal(t, model.StatusOpen, got.Status)
	require.NotNil(t, got.Broken)
	require.Equal(t, []string{"refs/heads/isu/ISU-7f3akq"}, got.Elsewhere,
		"the branch is still where a reader should look")

	require.NotNil(t, got.Issue, "and the branch's copy is what there is to render")
	require.Equal(t, "ISU-7f3akq", got.Issue.ID)
}

// Deriving `done` from a branch's copy of a file trunk cannot read would be a
// claim about trunk that trunk never made. Whatever the branch says, the answer
// is that nobody can tell.
func TestABranchSayingResolvedCannotFinishABrokenTrunkIssue(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		File("issues/ISU-7f3akq/README.md", "prose, no frontmatter\n").Commit("break it").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim it").
		Checkout(gittest.DefaultBranch)

	got := item(t, derive(t, r), "ISU-7f3akq")

	require.NotEqual(t, model.StatusDone, got.Status)
	require.Equal(t, model.StatusOpen, got.Status)
}

// A broken file names no parent and declares no type, so it is indexed as
// neither a child nor an epic — because there is nothing in it to read, not
// because it does not count. What matters is that the walk does not fall over
// on the way past.
func TestABrokenIssueIsSkippedByTheIndexRatherThanCrashingIt(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", epicIssue...).
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc")).
		Issue("ISU-39ka2p").Commit("an epic, a child, and a third issue").
		File("issues/ISU-39ka2p/README.md", "prose\n").Commit("break the third")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-40b1cc"))
	require.Equal(t, []string{"ISU-7f3akq"}, item(t, board, "ISU-40b1cc").Epic.Children,
		"the broken one names no parent, because nothing can read one out of it")
	require.Nil(t, item(t, board, "ISU-39ka2p").Epic)
	require.Empty(t, board.Children["ISU-39ka2p"])
}

// A repository with nothing wrong with it has an empty index rather than a nil
// one, so a caller may range over it without asking first.
func TestABoardWithNoBrokenFilesHasAnEmptyIndex(t *testing.T) {
	r := gittest.New(t).Issue("ISU-7f3akq").Commit("report ISU-7f3akq")

	board := derive(t, r)

	require.NotNil(t, board.Broken)
	require.Empty(t, board.Broken)
	require.Nil(t, item(t, board, "ISU-7f3akq").Broken)
}
