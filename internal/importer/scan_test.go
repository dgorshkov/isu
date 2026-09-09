package importer_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/importer"
)

// github is the key reader a GitHub import hands the scan: `#1234`, and
// nothing else. It is spelled here rather than imported so that the scan's
// tests are tests of the scan.
func githubKeys(text string) []string {
	var out []string

	for at := 0; at < len(text); at++ {
		if text[at] != '#' {
			continue
		}

		end := at + 1
		for end < len(text) && text[end] >= '0' && text[end] <= '9' {
			end++
		}

		if end > at+1 {
			out = append(out, text[at:end])
		}
	}

	return out
}

// mappingOf is the set of issues actually being imported, which is what every
// match is checked against.
func mappingOf(t *testing.T, keys ...string) *importer.Mapping {
	t.Helper()

	m := importer.NewMapping("ISU")

	for _, key := range keys {
		_, err := m.Add(key)
		require.NoError(t, err)
	}

	return m
}

func commit(oid, subject, body string, parents ...string) gitx.Commit {
	return gitx.Commit{
		OID:       oid,
		Parents:   parents,
		Subject:   subject,
		Body:      body,
		Committer: gitx.Signature{When: created},
	}
}

// M7-S3: a history deliberately mixing all three conventions plus a
// long tail of commits with no key, with the per-tier counts asserted exactly.
func TestTheThreeTiersAreCountedExactly(t *testing.T) {
	t.Parallel()

	m := mappingOf(t, "#1", "#2", "#3", "#4")

	commits := []gitx.Commit{
		commit("a1", "Tidy the makefile", ""),
		commit("a2", "Rework the loader", "This closes #1 for good."),
		commit("a3", "Merge pull request #456 from alice/fix-#2", "", "p1", "p2"),
		commit("a4", "Make the board render epics (#3)", ""),
		commit("a5", "Nothing to do with anything", "no keys here at all"),
		commit("a6", "Another tidy-up", ""),
		commit("a7", "Merge branch 'bug/#4-retries' into main", "", "p1", "p2"),
	}

	e := importer.NewEvidence()
	importer.Scan(e, commits, githubKeys, m)

	require.Equal(t, map[importer.Tier]int{
		importer.TierMessage:       1,
		importer.TierMergeBranch:   2,
		importer.TierSquashSubject: 1,
	}, e.Counts())

	require.Equal(t, 4, e.Len())

	for id, want := range map[string]struct {
		commit string
		tier   importer.Tier
	}{
		"ISU-1": {"a2", importer.TierMessage},
		"ISU-2": {"a3", importer.TierMergeBranch},
		"ISU-3": {"a4", importer.TierSquashSubject},
		"ISU-4": {"a7", importer.TierMergeBranch},
	} {
		link, ok := e.Link(id)
		require.Truef(t, ok, "%s has no link", id)
		require.Equal(t, want.commit, link.Commit)
		require.Equal(t, want.tier, link.Tier)
		require.Equal(t, created, link.When)
	}
}

// M7-S3: "assert a (#456) squash subject naming a pull request produces
// no link, and that the same subject does produce one when 456 is an imported
// issue."
func TestASquashSubjectNamingAPullRequestProducesNoLink(t *testing.T) {
	t.Parallel()

	const subject = "Make the board render epics (#456)"

	e := importer.NewEvidence()
	importer.Scan(e, []gitx.Commit{commit("a1", subject, "")}, githubKeys, mappingOf(t, "#1"))

	require.Zero(t, e.Len(),
		"issues and pull requests are numbered from one sequence, so a number that "+
			"is not one of these issues is not a link")

	e = importer.NewEvidence()
	importer.Scan(e, []gitx.Commit{commit("a1", subject, "")}, githubKeys, mappingOf(t, "#456"))

	link, ok := e.Link("ISU-456")
	require.True(t, ok)
	require.Equal(t, importer.TierSquashSubject, link.Tier)
}

func TestAMergeSubjectsOwnPullRequestNumberIsNotALink(t *testing.T) {
	t.Parallel()

	// `Merge pull request #456 from …` names a pull request by construction, so
	// the subject is read for the branch name it recorded and for nothing else.
	e := importer.NewEvidence()
	importer.Scan(e, []gitx.Commit{
		commit("a1", "Merge pull request #456 from alice/topic", "", "p1", "p2"),
	}, githubKeys, mappingOf(t, "#456"))

	require.Zero(t, e.Len())
}

// M7-S3: "an issue matched at two tiers records the stronger one".
func TestAnIssueMatchedAtTwoTiersRecordsTheStronger(t *testing.T) {
	t.Parallel()

	m := mappingOf(t, "#1")

	e := importer.NewEvidence()
	importer.Scan(e, []gitx.Commit{
		commit("a1", "Something about #1", ""),
		commit("a2", "Merge branch 'fix/#1' into main", "", "p1", "p2"),
		commit("a3", "Later still", "closes #1"),
		commit("a4", "Later again", "closes #1"),
	}, githubKeys, m)

	link, _ := e.Link("ISU-1")
	require.Equal(t, importer.TierMessage, link.Tier)
	require.Equal(t, "a3", link.Commit,
		"two commits at one tier resolve to the first of them, which is the oldest "+
			"in a walk that runs oldest first")
}

func TestTheSourcesOwnAnswerOutranksEveryTierOfTheScan(t *testing.T) {
	t.Parallel()

	// M7-S5: closing pull requests are the strongest tier there is, and
	// they arrive with the issue rather than through an integration.
	e := importer.NewEvidence()
	require.True(t, e.Record(importer.Link{
		ID: "ISU-1", Commit: "https://github.com/acme/widgets/pull/9",
		Tier: importer.TierClosing,
	}))

	importer.Scan(e, []gitx.Commit{commit("a1", "x", "closes #1")}, githubKeys, mappingOf(t, "#1"))

	link, _ := e.Link("ISU-1")
	require.Equal(t, importer.TierClosing, link.Tier)
	require.Equal(t, "https://github.com/acme/widgets/pull/9", link.Commit)

	require.False(t, e.Record(importer.Link{ID: "ISU-1", Tier: importer.TierMessage}))
}

func TestEveryTierHasAName(t *testing.T) {
	t.Parallel()

	for tier, want := range map[importer.Tier]string{
		importer.TierNone:          "none",
		importer.TierSquashSubject: "squash subject",
		importer.TierMergeBranch:   "merge branch name",
		importer.TierMessage:       "commit message",
		importer.TierClosing:       "closing pull request",
		importer.Tier(99):          "unknown",
	} {
		require.Equal(t, want, tier.String())
	}
}

func TestAMergeSubjectThatNamesNoBranchIsNotAMergeBranchMatch(t *testing.T) {
	t.Parallel()

	e := importer.NewEvidence()
	importer.Scan(e, []gitx.Commit{
		commit("a1", "Merge pull request #456 from ", "", "p1", "p2"),
		commit("a2", "Merge branch 'unterminated", "", "p1", "p2"),
		commit("a3", "Merged everything, somehow", "", "p1", "p2"),
	}, githubKeys, mappingOf(t, "#1"))

	require.Zero(t, e.Len())
}

func TestAKeyThatIsNotInTheImportIsDiscarded(t *testing.T) {
	t.Parallel()

	e := importer.NewEvidence()
	importer.Scan(e, []gitx.Commit{
		commit("a1", "See #1234 for the background", "and #9 while you are there"),
	}, githubKeys, mappingOf(t, "#7"))

	require.Zero(t, e.Len(),
		"`#1234` turns up in prose about nothing at all, and that is the whole "+
			"reason this scan cannot run before the issue list is in hand")
}

func TestALinkCarriesTheCommitDate(t *testing.T) {
	t.Parallel()

	when := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	one := commit("a1", "x", "closes #1")
	one.Committer.When = when

	e := importer.NewEvidence()
	importer.Scan(e, []gitx.Commit{one}, githubKeys, mappingOf(t, "#1"))

	link, _ := e.Link("ISU-1")
	require.Equal(t, when, link.When)
}
