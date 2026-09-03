package github_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/importer/github"
	"github.com/dgorshkov/isu/internal/issue"
)

// The content half: comments, field values, attachment links, and what closed
// each issue. Everything here reads a recorded dump — PLAN.md M7-S5 asks for a
// transcript and never the live API, because an importer tested against a
// service that changes underneath it fails for reasons nobody controls.

// PLAN.md M7-S5: "forty custom field values produce clean frontmatter and a
// complete source.yml" — here the two shapes a field value arrives in.
func TestIssueFieldValuesJoinEverythingElseUnmapped(t *testing.T) {
	t.Parallel()

	plan := planned(t, load(t, "dump.json", github.Options{}), importer.Options{})
	source := body(t, folder(t, plan, "ISU-1"), importer.SourceFileName)

	// The API's own spelling, then the two shorter ones a dump assembled by
	// hand tends to use.
	require.Contains(t, source, "Severity: high")
	require.Contains(t, source, "Team: platform")
	require.Contains(t, source, "Sprint: 14")

	// A select field's answer is not in `value` at all: it is the option's
	// name, and the id and the colour are GitHub's rather than this
	// repository's.
	require.Contains(t, source, "Stage: in review")
	require.Contains(t, source, "- cli")
	require.Contains(t, source, "- tui")

	require.NotContains(t, body(t, folder(t, plan, "ISU-1"), issue.ReadmeName), "Severity")
}

func TestAnAttachmentLinkIsListedOnceAndTheBodySurvives(t *testing.T) {
	t.Parallel()

	const link = "https://github.com/user-attachments/assets/0f1e2d3c"

	b := load(t, "dump.json", github.Options{})
	one := item(t, b, "#1")

	require.Equal(t, []string{link}, one.Attachments, "the same link twice is one attachment")
	require.Contains(t, one.Body, "![trace]("+link+")")

	plan := planned(t, b, importer.Options{})
	require.Contains(t, body(t, folder(t, plan, "ISU-1"), issue.ReadmeName), "("+link+")")
	require.Contains(t, b.Notes[1], "recorded, not fetched")
}

func TestCommentsBecomeFilesNamedForTheirAuthorAndDay(t *testing.T) {
	t.Parallel()

	plan := planned(t, load(t, "dump.json", github.Options{}), importer.Options{})
	one := folder(t, plan, "ISU-1")

	var comments []string

	for _, f := range one.Files {
		if f.Name != issue.ReadmeName && f.Name != importer.SourceFileName {
			comments = append(comments, f.Name)
		}
	}

	require.Equal(t, []string{
		"comments/2026-03-05-alice-o-hara-01.md",
		"comments/2026-03-05-alice-o-hara-02.md",
		"comments/2026-03-06-anon-01.md",
	}, comments)
}

// PLAN.md M7-S5: closing pull requests outrank the M7-S3 scan, and here they
// are free.
func TestWhatClosedAnIssueArrivesWithTheIssue(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})

	require.Equal(t, "https://github.com/acme/widgets/pull/456", item(t, b, "#3").Closing)
	require.Equal(t, "9e1f0a4c", item(t, b, "#5").Closing,
		"the closed timeline event's commit, where a commit closed one directly")
	require.Empty(t, item(t, b, "#1").Closing)
}

func TestAnAttachmentLinkAtTheVeryEndOfABodyIsStillFound(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})

	require.Equal(t, []string{"https://github.com/user-attachments/assets/aabbccdd"},
		item(t, b, "#10").Attachments)
}

func TestARepositoryThatUsesNoneOfItSaysSoAboutAttachmentsAnyway(t *testing.T) {
	t.Parallel()

	// The note is unconditional: a reader has to be told that resolving an
	// attachment still needs github.com whether or not this repository has one.
	b := load(t, "plain.json", github.Options{})

	require.Len(t, b.Notes, 1)
	require.Contains(t, b.Notes[0], "recorded, not fetched")
}

func TestAClosingPullRequestWithNoURLIsStillANumber(t *testing.T) {
	t.Parallel()

	b := load(t, "nameless.json", github.Options{})

	require.Equal(t, "#99", item(t, b, "#5").Closing)
}

func TestTheContentHalfIsReportedAsFoundToo(t *testing.T) {
	t.Parallel()

	// A repository with none of it still imports, and the dry run says which of
	// it it found — comments and issue fields included.
	require.Subset(t, load(t, "dump.json", github.Options{}).Found,
		[]string{"comments", "issue fields"})
}
