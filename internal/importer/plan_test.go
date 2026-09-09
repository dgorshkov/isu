package importer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/issue"
)

func TestEveryMappedIssueValidates(t *testing.T) {
	t.Parallel()

	plan := mapped(t, batch(), importer.Options{})

	for _, f := range plan.Folders {
		require.NoErrorf(t, f.Issue.Validate(), "%s", f.ID)
	}

	require.Equal(t, 2, plan.Report.Items)
	require.Equal(t, 2, plan.Report.Folders)
	require.Equal(t, map[string]int{"bug": 1, "story": 1}, plan.Report.Types)
	require.Equal(t, map[string]int{"open": 1, "resolved": 1}, plan.Report.States)
}

// M7-S4: the required field is written as a provenance line, and the
// dry run counts them so nobody mistakes archaeology for content.
func TestTheFieldATypeRequiresIsWrittenAsProvenance(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items = append(b.Items, importer.Item{
		Key: "#9", Ref: "acme/widgets#9", Title: "How much does contention cost?",
		Type: issue.TypeSpike, State: issue.StateOpen, Owner: "dmitry", Created: created,
	}, importer.Item{
		Key: "#10", Ref: "acme/widgets#10", Title: "Upgrade the linter",
		Type: issue.TypeChore, State: issue.StateOpen, Owner: "dmitry", Created: created,
	})

	plan := mapped(t, b, importer.Options{})

	require.Equal(t, "imported from acme/widgets PROJ-1234; see the body",
		find(t, plan, "PROJ-1234").Issue.Repro)
	require.Equal(t, "imported from acme/widgets#7; see the body",
		find(t, plan, "ISU-7").Issue.Acceptance)
	require.Equal(t, "imported from acme/widgets#9; see the body",
		find(t, plan, "ISU-9").Issue.Question)
	require.Empty(t, find(t, plan, "ISU-10").Issue.Repro,
		"a chore requires nothing, so nothing is invented for it")

	require.Equal(t, 3, plan.Report.Provenance)
}

func TestAnUnassignedIssueTakesTheFallbackOwner(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[0].Owner = ""

	plan := mapped(t, b, importer.Options{Owner: "support"})
	require.Equal(t, "support", find(t, plan, "PROJ-1234").Issue.Owner)
	require.Zero(t, plan.Report.Unowned)
}

// M7-S4: "failing that the import refuses and the dry run says how many
// issues have neither".
func TestAnIssueWithNoOwnerAtAllIsCountedAndNotWritten(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[0].Owner = ""

	plan := mapped(t, b, importer.Options{})

	require.Equal(t, 1, plan.Report.Unowned)
	require.Equal(t, []string{"ISU-7"}, ids(plan))
	require.Len(t, plan.Report.Skipped, 1)
	require.Contains(t, plan.Report.Skipped[0].Why, "--owner")
}

// M7-S4: "assert an issue whose milestone was skipped emits no parent",
// and the same for a blocker that was skipped.
func TestALinkToAnIssueThatIsNotWrittenIsRecordedAndNotWritten(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items = append(b.Items, importer.Item{
		Key: "#M4", Ref: "acme/widgets#M4", Title: "Make login reliable",
		Type: issue.TypeEpic, Epic: true, Created: created,
	})
	b.Items[0].Parent = "#M4"
	b.Items[0].BlockedBy = []string{"#7", "#999"}

	plan := mapped(t, b, importer.Options{})

	require.Equal(t, []string{"PROJ-1234", "ISU-7"}, ids(plan),
		"the epic has no owner and no --owner, so it is not written")

	one := find(t, plan, "PROJ-1234")
	require.Empty(t, one.Issue.Parent,
		"a parent naming a folder that does not exist is exactly what M5-S2 fails on")
	require.Equal(t, []string{"ISU-7"}, one.Issue.BlockedBy)

	source := file(t, one, importer.SourceFileName)
	require.Contains(t, source, "#M4")
	require.Contains(t, source, "#999")
	require.Equal(t, 2, plan.Report.Dangling)
}

func TestAnEpicIsWrittenWithNoState(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items = append(b.Items, importer.Item{
		Key: "#M4", Ref: "acme/widgets#M4", Title: "Make login reliable",
		Type: issue.TypeEpic, Epic: true, State: issue.StateResolved,
		Owner: "dmitry", Created: created,
		Extra: map[string]any{"milestone_state": "closed"},
	})
	b.Items[0].Parent = "#M4"

	plan := mapped(t, b, importer.Options{})
	epic := find(t, plan, "ISU-M4")

	require.Empty(t, epic.Issue.State,
		"an epic's status is the fold over its children, and the fold is what decides")
	require.NotContains(t, file(t, epic, issue.ReadmeName), "state:")
	require.Contains(t, file(t, epic, importer.SourceFileName), "milestone_state: closed")

	require.Equal(t, "ISU-M4", find(t, plan, "PROJ-1234").Issue.Parent)
	require.Equal(t, 1, plan.Report.Epics)
	require.Equal(t, 1, plan.Report.States["(fold over its children)"])
}

// The data model: comment files are <date>-<author>-<nn>.md, and the
// sequence is not decoration — without it a second comment by the same person
// on the same day silently overwrites the first.
func TestThreeCommentsByOnePersonOnOneDayAreThreeFiles(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC)

	b := batch()
	b.Items[0].Comments = []importer.Comment{
		{Author: "Alice O'Hara", When: day, Body: "first"},
		{Author: "Alice O'Hara", When: day.Add(time.Hour), Body: "second"},
		{Author: "Alice O'Hara", When: day.Add(2 * time.Hour), Body: "third"},
		{Author: "Alice O'Hara", When: day.AddDate(0, 0, 1), Body: "the next day"},
		{Author: "", When: day, Body: "nobody"},
	}

	plan := mapped(t, b, importer.Options{})
	folder := find(t, plan, "PROJ-1234")

	require.Equal(t, []string{
		issue.ReadmeName,
		importer.SourceFileName,
		"comments/2026-08-24-alice-o-hara-01.md",
		"comments/2026-08-24-alice-o-hara-02.md",
		"comments/2026-08-24-alice-o-hara-03.md",
		"comments/2026-08-25-alice-o-hara-01.md",
		"comments/2026-08-24-anon-01.md",
	}, names(folder))

	require.Equal(t, 5, plan.Report.Comments)
	require.Contains(t, file(t, folder, "comments/2026-08-24-alice-o-hara-02.md"), "second")
}

// M7-S5: "a comment containing frontmatter delimiters does not corrupt
// the issue file".
func TestACommentFullOfFrontmatterDelimitersCorruptsNothing(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[0].Comments = []importer.Comment{{
		Author: "mallory",
		When:   created,
		Body:   "---\nowner: mallory\nstate: resolved\n---\n\nnot an issue file",
	}}

	plan := mapped(t, b, importer.Options{})
	folder := find(t, plan, "PROJ-1234")

	readme := file(t, folder, issue.ReadmeName)
	require.Equal(t, "dmitry", folder.Issue.Owner)
	require.NotContains(t, readme, "mallory")

	body := file(t, folder, "comments/2026-03-04-mallory-01.md")
	require.Contains(t, body, "owner: mallory", "the comment is kept as it was written")
	require.True(t, strings.HasPrefix(body, "**mallory** on 2026-03-04\n\n"),
		"a comment file opens with who wrote it, so its first line is never a delimiter")
}

// M7-S5: "an attachment link survives the body byte for byte and is
// listed".
func TestAnAttachmentLinkSurvivesTheBodyAndIsListed(t *testing.T) {
	t.Parallel()

	const link = "https://github.com/user-attachments/assets/0f1e2d3c"

	b := batch()
	b.Items[0].Body = "Here is the trace:\n\n![trace](" + link + ")\n"
	b.Items[0].Attachments = []string{link}

	plan := mapped(t, b, importer.Options{})
	folder := find(t, plan, "PROJ-1234")

	require.Contains(t, file(t, folder, issue.ReadmeName), "("+link+")")
	require.Contains(t, file(t, folder, importer.SourceFileName), link)
	require.Equal(t, 1, plan.Report.Attachments)
}

func TestAnEmptyBodyWritesNoBody(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[0].Body = "\n\n"

	readme := file(t, find(t, mapped(t, b, importer.Options{}), "PROJ-1234"), issue.ReadmeName)
	require.True(t, strings.HasSuffix(readme, "---\n"), "no body is no body: %q", readme)
}

func TestATitleThatIsNotOneLineIsMadeIntoOne(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[0].Title = "  Login retries\r\ndrop the second attempt  "
	b.Items[1].Title = ""

	plan := mapped(t, b, importer.Options{})

	require.Equal(t, "Login retries  drop the second attempt",
		find(t, plan, "PROJ-1234").Issue.Title)
	require.Equal(t, "acme/widgets#7", find(t, plan, "ISU-7").Issue.Title,
		"a source that hands over no title still has to produce a valid issue")
}

func TestAnItemThatCannotBeAValidIssueStopsTheImport(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[0].Created = time.Time{}

	m, err := importer.Keys(b, "ISU")
	require.NoError(t, err)

	_, err = importer.Map(b, m, importer.Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "would not be a valid issue")
	require.Contains(t, err.Error(), "created")
}

func TestTheDryRunShowsSomeIssuesInFull(t *testing.T) {
	t.Parallel()

	plan := mapped(t, batch(), importer.Options{Samples: 1})

	require.Len(t, plan.Report.Samples, 1)
	require.Equal(t, "PROJ-1234", plan.Report.Samples[0].ID)
	require.Equal(t, "PROJ-1234", plan.Report.Samples[0].Key)
	require.Contains(t, plan.Report.Samples[0].Files, importer.SourceFileName)
	require.Contains(t, plan.Report.Samples[0].README, "id: PROJ-1234")

	require.Len(t, mapped(t, batch(), importer.Options{}).Report.Samples,
		importer.DefaultSamples)
}

func TestWhatTheSourceSaidForItselfSurvivesIntoTheReport(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Skipped = []importer.Skip{{Ref: "#3", Why: "a pull request"}}
	b.Unplaced = []string{"regression", "Epic"}
	b.Found = []string{"issue types"}
	b.Notes = []string{"attachment links are recorded, not fetched"}
	b.Requests = 12

	r := mapped(t, b, importer.Options{}).Report

	require.Equal(t, []importer.Skip{{Ref: "#3", Why: "a pull request"}}, r.Skipped)
	require.Equal(t, []string{"Epic", "regression"}, r.Unplaced, "sorted, so a run is a run")
	require.Equal(t, []string{"issue types"}, r.Found)
	require.Equal(t, 12, r.Requests)
	require.Equal(t, "fake", r.Source)
	require.Equal(t, "acme/widgets", r.Repository)
}

func TestTheEvidenceRecorderReachesSourceYAML(t *testing.T) {
	t.Parallel()

	e := importer.NewEvidence()
	e.Record(importer.Link{
		ID: "ISU-7", Commit: "https://github.com/acme/widgets/pull/456",
		Tier: importer.TierClosing,
	})

	plan := mapped(t, batch(), importer.Options{Evidence: e})

	source := file(t, find(t, plan, "ISU-7"), importer.SourceFileName)
	require.Contains(t, source, "resolved_by: https://github.com/acme/widgets/pull/456")
	require.Contains(t, source, "resolved_by_evidence: closing pull request")

	require.NotContains(t, file(t, find(t, plan, "PROJ-1234"), importer.SourceFileName),
		"resolved_by",
		"an unlinked issue imports with its resolution date only: a link nobody can "+
			"check is worse than no link")

	require.Equal(t, map[string]int{"closing pull request": 1}, plan.Report.Evidence)
}

func TestABlockerThatTheImportRefusedIsRecordedAndNotWritten(t *testing.T) {
	t.Parallel()

	// The other half of the pruning rule: this key *is* in the import, so the
	// mapping resolves it — and then the issue behind it is refused for want of
	// an owner, which leaves an id naming a folder nothing will write.
	b := batch()
	b.Items[1].Owner = ""
	b.Items[0].BlockedBy = []string{"#7"}
	b.Items[0].Parent = "#404"

	plan := mapped(t, b, importer.Options{})

	require.Equal(t, []string{"PROJ-1234"}, ids(plan))

	one := find(t, plan, "PROJ-1234")
	require.Empty(t, one.Issue.BlockedBy)
	require.Empty(t, one.Issue.Parent)

	source := file(t, one, importer.SourceFileName)
	require.Contains(t, source, "#7")
	require.Contains(t, source, "#404")
	require.Equal(t, 2, plan.Report.Dangling)
}

func TestWhatTheSourceSaysClosedAnIssueReachesSourceYAML(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[1].Closing = "https://github.com/acme/widgets/pull/456"

	source := file(t, find(t, mapped(t, b, importer.Options{}), "ISU-7"),
		importer.SourceFileName)
	require.Contains(t, source, "closed_by: https://github.com/acme/widgets/pull/456")
}

func TestAValueNothingCanWriteStopsTheImportRatherThanHalfWritingIt(t *testing.T) {
	t.Parallel()

	item := custom(0)
	item.Extra["unwritable"] = make(chan int)

	b := batchOf(item)

	m, err := importer.Keys(b, "ISU")
	require.NoError(t, err)

	_, err = importer.Map(b, m, importer.Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), importer.SourceFileName)
	require.Contains(t, err.Error(), "acme/widgets#1")
}
