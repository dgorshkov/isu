package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

func TestReadyIsJSONByDefaultAndOneIssuePerLine(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := isu(t, r.Dir(), "ready").ok(t)

	issues := decodeLines[Issue](t, got)
	require.NotEmpty(t, issues)

	// `isu ready --json | head -1` has to be the top of the queue rather than
	// an opening brace, which is the whole reason the format is one value per
	// line and not one indented document.
	first := decodeLines[Issue](t, result{stdout: firstLine(got.stdout)})
	require.Len(t, first, 1)
	require.Equal(t, issues[0].ID, first[0].ID)
}

func TestReadyGivesAnAgentEnoughToStart(t *testing.T) {
	t.Parallel()

	r := board(t)

	issues := decodeLines[Issue](t, isu(t, r.Dir(), "ready").ok(t))

	top := issues[0]
	require.Equal(t, "ISU-openly", top.ID)
	require.Equal(t, "Login retries drop the second attempt", top.Title)
	require.Equal(t, "bug", top.Type)
	require.Equal(t, "p1", top.Priority)
	require.Equal(t, "post twice", top.Repro, "a bug is not startable without its repro")
	require.Equal(t, "dmitry", top.Owner)
	require.Empty(t, top.BlockedBy)
}

func TestReadyExcludesWhatNobodyShouldPickUp(t *testing.T) {
	t.Parallel()

	r := board(t)

	var got []string
	for _, item := range decodeLines[Issue](t, isu(t, r.Dir(), "ready").ok(t)) {
		got = append(got, item.ID)
	}

	// open and reopened are both trunk saying open with nobody on it. Done and
	// dropped are terminal, in progress means somebody is already on it, a
	// report nobody has accepted is not work anybody agreed to, and an epic is
	// not a thing anybody can pick up.
	require.Equal(t, []string{"ISU-openly", "ISU-reopen"}, got)
}

func TestReadyExcludesABlockedIssueAndIncludesAnUnblockedOne(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-blockr", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("The blocker")).
		Issue("ISU-waiter", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Waits on it"), gittest.BlockedBy("ISU-blockr")).
		Issue("ISU-ghostb", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Waits on nothing that exists"), gittest.BlockedBy("ISU-nobody")).
		Commit("a blocker and two waiters")

	require.Equal(t, []string{"ISU-blockr"}, readyIDs(t, r.Dir()))

	// A blocker that resolves at trunk stops blocking.
	r.Issue("ISU-blockr", gittest.Owner("dmitry"), gittest.Type("chore"),
		gittest.Title("The blocker"), gittest.State("resolved")).
		Commit("resolve the blocker")

	require.Equal(t, []string{"ISU-waiter"}, readyIDs(t, r.Dir()),
		"a blocker naming nothing this repository has still blocks: that is isu "+
			"check's to report, and until somebody does it is not a reason to hand work out")
}

func TestReadyTreatsAnEpicBlockerAsItsRollup(t *testing.T) {
	t.Parallel()

	// An epic has no state of its own, so "is my blocker finished" is a
	// question about its children.
	r := configured(t).
		Issue("ISU-halfep", gittest.Type("epic"), gittest.Without("state"),
			gittest.Owner("dmitry"), gittest.Title("Half done")).
		Issue("ISU-halfkd", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Parent("ISU-halfep"), gittest.Title("A child that is not done")).
		Issue("ISU-fullep", gittest.Type("epic"), gittest.Without("state"),
			gittest.Owner("dmitry"), gittest.Title("All done")).
		Issue("ISU-fullkd", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Parent("ISU-fullep"), gittest.Title("A child that is done"),
			gittest.State("resolved")).
		Issue("ISU-onhalf", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Blocked by the half-done epic"), gittest.BlockedBy("ISU-halfep")).
		Issue("ISU-onfull", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Blocked by the finished epic"), gittest.BlockedBy("ISU-fullep")).
		Commit("two epics and two things waiting on them")

	got := readyIDs(t, r.Dir())

	require.Contains(t, got, "ISU-onfull", "a fully resolved epic is a finished blocker")
	require.NotContains(t, got, "ISU-onhalf", "a half-done epic is not")
	require.NotContains(t, got, "ISU-halfep", "and an epic is never itself the work")
}

func TestReadyPutsAP0AheadOfAnOlderP2(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-oldp2x", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Created("2019-01-01"), gittest.Title("Ancient and unimportant")).
		Issue("ISU-newp0x", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Created("2026-08-30"), gittest.Priority("p0"),
			gittest.Title("New and on fire")).
		Commit("two issues")

	require.Equal(t, []string{"ISU-newp0x", "ISU-oldp2x"}, readyIDs(t, r.Dir()))
}

func TestReadyExcludesAnIssueTrunkCannotDecode(t *testing.T) {
	t.Parallel()

	r := configured(t).
		File("issues/ISU-broken/README.md", "not an issue\n").
		Commit("a half-written issue")

	require.Empty(t, readyIDs(t, r.Dir()),
		"handing an agent an issue nobody can read is handing it a parse error")
}

func TestReadyAsTextWhenAskedForIt(t *testing.T) {
	t.Parallel()

	r := board(t)

	golden(t, "ready/text.txt", isu(t, r.Dir(), "ready", "--json=false").ok(t).stdout)
}

func TestReadyWithNothingToDoSaysSo(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "ready", "--json=false").ok(t)

	require.Contains(t, got.stdout, "nothing is ready")

	empty := isu(t, r.Dir(), "ready").ok(t)
	require.Empty(t, empty.stdout, "an empty queue is no lines, not an empty array")
}

func readyIDs(t *testing.T, dir string) []string {
	t.Helper()

	var ids []string
	for _, item := range decodeLines[Issue](t, isu(t, dir, "ready").ok(t)) {
		ids = append(ids, item.ID)
	}

	return ids
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i+1]
		}
	}

	return s
}
