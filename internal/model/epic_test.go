package model_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// M3-S2. An epic declares no state; its status is the fold over its children.
//
// The cycle cases are first because one of them blew a stack during
// prototyping. An epic that is its own parent is a file one person can write by
// hand, and a derivation that recurses forever on it takes the whole board with
// it.

func TestAnEpicThatIsItsOwnParentStillReturnsAValue(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", append(epicIssue, gittest.Parent("ISU-40b1cc"))...).
		Commit("an epic that is its own parent")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-40b1cc"))
	require.True(t, item(t, board, "ISU-40b1cc").Epic.Cycle,
		"the rollup met the epic again while folding it, and says so")
}

func TestATwoNodeParentCycleStillReturnsAValue(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", append(epicIssue, gittest.Parent("ISU-39ka2p"))...).
		Issue("ISU-39ka2p", append(epicIssue, gittest.Parent("ISU-40b1cc"))...).
		Commit("two epics pointing at each other")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-40b1cc"))
	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-39ka2p"))
	require.True(t,
		item(t, board, "ISU-40b1cc").Epic.Cycle || item(t, board, "ISU-39ka2p").Epic.Cycle,
		"one of them is where the walk came back round")
}

func TestAnEpicWithMixedChildrenIsOpen(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", epicIssue...).
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc"), gittest.State("resolved")).
		Issue("ISU-39ka2p", gittest.Parent("ISU-40b1cc")).
		Commit("an epic with one child done and one open")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-40b1cc"))
	require.Equal(t, []string{"ISU-39ka2p", "ISU-7f3akq"},
		item(t, board, "ISU-40b1cc").Epic.Children, "children come out in id order")
}

func TestAnEpicWhoseChildrenAreAllTerminalIsDone(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", epicIssue...).
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc"), gittest.State("resolved")).
		Issue("ISU-39ka2p", append(droppedIssue, gittest.Parent("ISU-40b1cc"))...).
		Commit("one child resolved and one dropped")

	require.Equal(t, model.StatusDone, statusOf(t, derive(t, r), "ISU-40b1cc"),
		"terminal is terminal, and only every child dropped makes the epic dropped")
}

func TestAnEpicWhoseChildrenAreAllDroppedIsDropped(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", epicIssue...).
		Issue("ISU-7f3akq", append(droppedIssue, gittest.Parent("ISU-40b1cc"))...).
		Issue("ISU-39ka2p", append(droppedIssue, gittest.Parent("ISU-40b1cc"))...).
		Commit("every child dropped")

	require.Equal(t, model.StatusDropped, statusOf(t, derive(t, r), "ISU-40b1cc"))
}

// A child being worked on is not a terminal child, so the epic above it is
// open. There is no `in progress` epic: an epic is not claimable, because it
// has no state to flip.
func TestAnEpicWithAClaimedChildIsOpen(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", epicIssue...).
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc")).
		Commit("an epic and its child").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc"), gittest.State("resolved")).
		Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	board := derive(t, r)

	require.Equal(t, model.StatusInProgress, statusOf(t, board, "ISU-7f3akq"))
	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-40b1cc"))
}

func TestNestedEpicsFoldThreeDeep(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-000top", epicIssue...).
		Issue("ISU-00mid", append(epicIssue, gittest.Parent("ISU-000top"))...).
		Issue("ISU-0low", append(epicIssue, gittest.Parent("ISU-00mid"))...).
		Issue("ISU-7f3akq", gittest.Parent("ISU-0low"), gittest.State("resolved")).
		Commit("three epics over one resolved issue")

	board := derive(t, r)

	for _, id := range []string{"ISU-0low", "ISU-00mid", "ISU-000top"} {
		require.Equal(t, model.StatusDone, statusOf(t, board, id),
			"%s folds what is under it", id)
	}
}

func TestANestedEpicIsOnlyDoneWhenEverythingUnderItIs(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-000top", epicIssue...).
		Issue("ISU-00mid", append(epicIssue, gittest.Parent("ISU-000top"))...).
		Issue("ISU-7f3akq", gittest.Parent("ISU-00mid"), gittest.State("resolved")).
		Issue("ISU-39ka2p", gittest.Parent("ISU-000top")).
		Commit("a finished sub-epic beside an open issue")

	board := derive(t, r)

	require.Equal(t, model.StatusDone, statusOf(t, board, "ISU-00mid"))
	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-000top"))
}

// An epic with no children is a check failure, not a status. The board still
// has to render it, so it reads open and says what is wrong with it.
func TestAnEpicWithNoChildrenIsOpenAndSaysItIsEmpty(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", epicIssue...).Commit("an epic nobody has filled in")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-40b1cc"))
	require.True(t, item(t, board, "ISU-40b1cc").Epic.Empty())
	require.Empty(t, item(t, board, "ISU-40b1cc").Epic.Children)
}

// `parent:` must name an epic, and that is M5-S2's to report: it is a
// repository-level rule, and this package is not where an issue is validated.
// What matters here is that the index does not lose the fact.
func TestAParentThatIsNotAnEpicIsIndexedAndNotFolded(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc").
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc"), gittest.State("resolved")).
		Commit("a child pointing at something that is not an epic")

	board := derive(t, r)

	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-40b1cc"),
		"an ordinary issue takes its status from its own state, whoever points at it")
	require.Nil(t, item(t, board, "ISU-40b1cc").Epic)
	require.Equal(t, []string{"ISU-7f3akq"}, board.Children["ISU-40b1cc"],
		"the index keeps it, so that isu check can report it")
}

func TestAParentThatDoesNotExistIsIndexedAndNothingElse(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq", gittest.Parent("ISU-nobody")).
		Commit("a child pointing at nothing")

	board := derive(t, r)

	require.Equal(t, []string{"ISU-7f3akq"}, board.Children["ISU-nobody"])
	require.NotContains(t, board.IDs(), "ISU-nobody",
		"a dangling parent is a reference, not an issue")
}

// An epic reported on a branch that trunk has never seen is awaiting triage
// like any other report. Folding its children would answer a question nobody
// asked while hiding the one that matters.
func TestAnEpicOnlyOnABranchIsAwaitingTriage(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("report/ISU-40b1cc").Checkout("report/ISU-40b1cc").
		Issue("ISU-40b1cc", epicIssue...).Commit("report an epic").
		Checkout(gittest.DefaultBranch)

	require.Equal(t, model.StatusAwaitingTriage, statusOf(t, derive(t, r), "ISU-40b1cc"))
}

// The gate PLAN.md M3-S2 sets. Every fifth issue is an epic owning the four
// before it, so 5,000 issues is a thousand epics — ten times what the story
// asks for, against the same budget.
//
// The board is built in memory rather than generated as a repository and read
// back, and that is not a shortcut. What this story measures is the fold, which
// is a pure function over issues that are already loaded; generating 5,000
// issue folders and loading them is half a minute of git doing work this test
// does not time. It is also half a minute spent on a CI runner that is at that
// moment timing internal/repo's read path in another process — which is how
// this test made M2-S5's gate fail twice on macOS before anybody noticed the
// two were fighting. The issues are still parsed and decoded exactly as a
// loader would hand them over; only git is gone.
func TestRollupOverFiveThousandIssuesIsFast(t *testing.T) {
	const (
		budget = 50 * time.Millisecond
		issues = 5000
	)

	in := model.Input{
		Loaded:  generatedBoard(t, issues),
		History: repo.History{},
		Config:  defaultConfig(),
		Now:     time.Now(),
	}

	started := time.Now()
	board := model.Derive(in)
	took := time.Since(started)

	var epics, children int

	for _, id := range board.IDs() {
		if epic := board.Items[id].Epic; epic != nil {
			epics++
			children += len(epic.Children)
		}
	}

	require.Equal(t, issues, len(board.Items))
	require.Equal(t, 1000, epics)
	require.Equal(t, 4000, children, "every epic folded the four issues under it")
	require.Less(t, took, budget,
		"deriving %d issues with %d epics took %s, and the budget is %s",
		len(board.Items), epics, took, budget)
}

// generatedBoard builds a trunk-only board of the given size, shaped like the
// fixture gittest.Generate writes: the five types in turn, every fifth one an
// epic, and everything else naming the epic that closes its block of five.
func generatedBoard(t *testing.T, issues int) *repo.Board {
	t.Helper()

	set := &repo.Set{Issues: make(map[string]*issue.Issue, issues)}

	for n := range issues {
		id := gittest.GeneratedID(gittest.DefaultPrefix, n)

		doc, err := issue.Parse([]byte(generatedIssue(id, n, issues)))
		require.NoError(t, err)

		decoded, err := issue.Decode(doc)
		require.NoError(t, err)

		decoded.Folder = id
		require.NoError(t, decoded.Validate(), "the fixture must be a repository somebody could have")

		set.Issues[id] = decoded
	}

	return &repo.Board{
		Trunk:   set,
		Refs:    map[string]*repo.Set{},
		Changed: map[string][]string{},
	}
}

// generatedIssue renders the nth issue file of a set of that many.
func generatedIssue(id string, n, issues int) string {
	var b strings.Builder

	kind := []string{"chore", "story", "bug", "spike", "epic"}[n%5]

	fmt.Fprintf(&b, "---\nschema: 1\nid: %s\ntitle: Generated issue %d\ntype: %s\n", id, n, kind)

	// An epic declares no state, and takes its status from its children.
	if kind != "epic" {
		b.WriteString("state: open\n")
	}

	fmt.Fprintf(&b, "owner: %s\ncreated: 2026-08-24\npriority: p%d\n",
		[]string{"dmitry", "sam", "alex"}[n%3], n%4)

	switch kind {
	case "bug":
		b.WriteString("repro: run it twice\n")
	case "story":
		b.WriteString("acceptance: the board renders it\n")
	case "spike":
		b.WriteString("question: which way round is this\n")
	}

	// A parent that is not in the set would be a dangling reference, which is
	// M5-S2's to report and not a fixture's to write.
	if epic := n - n%5 + 4; kind != "epic" && epic < issues {
		fmt.Fprintf(&b, "parent: %s\n", gittest.GeneratedID(gittest.DefaultPrefix, epic))
	}

	fmt.Fprintf(&b, "---\n\nGenerated for a fixture.\n\nThis is issue %d.\n", n)

	return b.String()
}
