package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

func TestShowRendersAnIssueInFull(t *testing.T) {
	t.Parallel()

	r := board(t)

	golden(t, "show/open.txt", isu(t, r.Dir(), "show", "ISU-openly").ok(t).stdout)
	golden(t, "show/claimed.txt", isu(t, r.Dir(), "show", "ISU-inprog").ok(t).stdout)
	golden(t, "show/epic.txt", isu(t, r.Dir(), "show", "ISU-epical").ok(t).stdout)
	golden(t, "show/dropped.txt", isu(t, r.Dir(), "show", "ISU-dropit").ok(t).stdout)
	golden(t, "show/reopened.txt", isu(t, r.Dir(), "show", "ISU-reopen").ok(t).stdout)
}

func TestShowJSONRoundTripsThroughTheDocumentedStruct(t *testing.T) {
	t.Parallel()

	r := board(t)

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))

	require.Equal(t, "ISU-openly", payload.Issue.ID)
	require.Equal(t, "bug", payload.Issue.Type)
	require.Equal(t, "open", payload.Issue.Status)
	require.Equal(t, "p1", payload.Issue.Priority)
	require.Equal(t, "The second POST is dropped.\n", payload.Body)

	require.NotNil(t, payload.Parent)
	require.Equal(t, "ISU-epical", payload.Parent.ID)
	require.Equal(t, "Make login reliable", payload.Parent.Title)
	require.True(t, payload.Parent.Known)
}

func TestShowRendersAnEpicsChildren(t *testing.T) {
	t.Parallel()

	r := board(t)

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-epical").ok(t))

	require.Len(t, payload.Children, 1)
	require.Equal(t, "ISU-openly", payload.Children[0].ID)

	got := isu(t, r.Dir(), "show", "ISU-epical").ok(t)
	require.Contains(t, got.stdout, "children")
	require.Contains(t, got.stdout, "ISU-openly")
}

func TestShowNamesTheClaimAndWhoMadeIt(t *testing.T) {
	t.Parallel()

	r := board(t)

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-inprog").ok(t))

	require.Len(t, payload.Issue.Claims, 1)
	require.Equal(t, "alice", payload.Issue.Claims[0].Claimant)
	require.False(t, payload.Issue.Claims[0].Stale)
	require.Positive(t, payload.Issue.Claims[0].AgeSeconds)

	got := isu(t, r.Dir(), "show", "ISU-inprog").ok(t)
	require.Contains(t, got.stdout, "claimed by alice on refs/heads/isu/ISU-inprog")
}

func TestShowReadsCommentsAndAttachmentsFromTheWorkingTree(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-withit", gittest.Owner("dmitry"), gittest.Title("Has things beside it"),
			gittest.Attachment("repro.har", "{}"),
			gittest.Comment("2026-08-24-support-01", "It happens on Safari too.\n"),
			gittest.Comment("2026-08-24-support-02", "And on Firefox.\n")).
		Commit("an issue with a folder")

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-withit").ok(t))

	require.Equal(t, []string{"repro.har"}, payload.Attachments)
	require.Len(t, payload.Comments, 2)
	require.Equal(t, "2026-08-24-support-01.md", payload.Comments[0].Name)
	require.Equal(t, "It happens on Safari too.\n", payload.Comments[0].Body)

	got := isu(t, r.Dir(), "show", "ISU-withit").ok(t)
	require.Contains(t, got.stdout, "attachments")
	require.Contains(t, got.stdout, "repro.har")
	require.Contains(t, got.stdout, "It happens on Safari too.")
}

func TestShowReadsAFolderThatIsOnlyOnABranch(t *testing.T) {
	t.Parallel()

	// A report nobody has checked out is not on disk, so its comments and
	// attachments have to come out of git.
	r := configured(t).
		Branch("report/ISU-onbrch").Checkout("report/ISU-onbrch").
		Issue("ISU-onbrch", gittest.Owner("support"), gittest.Title("Filed from a branch"),
			gittest.Attachment("screenshot.png", "not really a png"),
			gittest.Comment("2026-08-25-support-01", "Still happening.\n")).
		Commit("report ISU-onbrch").
		Checkout(gittest.DefaultBranch)

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-onbrch").ok(t))

	require.Equal(t, "awaiting triage", payload.Issue.Status)
	require.False(t, payload.Issue.OnTrunk)
	require.Equal(t, []string{"screenshot.png"}, payload.Attachments)
	require.Len(t, payload.Comments, 1)
	require.Equal(t, "Still happening.\n", payload.Comments[0].Body)
}

func TestShowAtARefReadsThatRefRatherThanTheWorkingTree(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-attach", gittest.Owner("dmitry"), gittest.Title("Grows an attachment")).
		Commit("no attachment yet")

	r.Issue("ISU-attach", gittest.Owner("dmitry"), gittest.Title("Grows an attachment"),
		gittest.Attachment("later.txt", "added afterwards")).
		Commit("add one")

	// --ref names an older commit, whose folder had nothing in it. The working
	// tree does, and must not be what is read.
	older := r.Git("rev-parse", "HEAD~1")

	payload := decode[ShowPayload](t,
		isu(t, r.Dir(), "--json", "--ref", older, "show", "ISU-attach").ok(t))

	require.Empty(t, payload.Attachments)

	current := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-attach").ok(t))
	require.Equal(t, []string{"later.txt"}, current.Attachments)
}

func TestShowSaysSoWhenThereIsNoSuchIssue(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := isu(t, r.Dir(), "show", "ISU-nobody")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "no issue ISU-nobody")
}

func TestShowRendersABlockerThatNamesNothing(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-blockd", gittest.Owner("dmitry"), gittest.Title("Waits on a ghost"),
			gittest.BlockedBy("ISU-ghosty")).
		Commit("an issue blocked by nothing that exists")

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-blockd").ok(t))

	require.Len(t, payload.Blockers, 1)
	require.False(t, payload.Blockers[0].Known,
		"an id that resolves to nothing is reported, not dropped: what to do about "+
			"it is isu check's call")

	got := isu(t, r.Dir(), "show", "ISU-blockd").ok(t)
	require.Contains(t, got.stdout, "no issue with this id")
}

func TestShowRendersAnIssueTrunkCannotDecode(t *testing.T) {
	t.Parallel()

	r := configured(t).
		File("issues/ISU-broken/README.md", "no frontmatter here\n").
		Commit("a half-written issue")

	got := isu(t, r.Dir(), "show", "ISU-broken").ok(t)

	require.Contains(t, got.stdout, "will not decode")

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-broken").ok(t))
	require.NotNil(t, payload.Issue.Broken)
}

func TestShowPrintsTheFreshnessLine(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := isu(t, r.Dir(), "show", "ISU-openly").ok(t)

	require.Contains(t, got.stdout, "no remote refs")
}
