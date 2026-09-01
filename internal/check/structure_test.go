package check_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/check"
	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// The rules themselves are asserted against real repositories, in
// internal/cli/structure_test.go: one repository per violation, loaded and
// derived by the product's own assembly rather than by a second one written for
// the tests. What is left here is what a repository is a clumsy way to say.

// rule is one registered check, by name.
func rule(t *testing.T, name string) check.Check {
	t.Helper()

	for _, c := range check.Checks.All() {
		if c.Name() == name {
			return c
		}
	}

	t.Fatalf("no check called %q is registered", name)

	return nil
}

// A check is handed whatever the caller loaded, and a caller that loaded
// nothing is not a caller to panic on: `isu check` in a repository with no
// commits reaches every one of these with an empty board.
func TestEveryCheckSurvivesAnInputWithNothingInIt(t *testing.T) {
	t.Parallel()

	for _, c := range check.Checks.All() {
		require.Emptyf(t, c.Run(check.Input{}), "%s found something in nothing", c.Name())
	}
}

// The cap has a default, and a caller who has not loaded a configuration gets
// it rather than a cap of nothing — the same courtesy model.Derive extends to
// stale_days, and for the same reason: a zero value that silently fails every
// row is a footgun.
func TestTheAttachmentCapFallsBackToTheDefault(t *testing.T) {
	t.Parallel()

	files := repo.Files{"ISU-7f3akq": []repo.File{
		{ID: "ISU-7f3akq", Name: "small.bin", Path: "issues/ISU-7f3akq/small.bin", Size: 16},
	}}

	require.Empty(t, rule(t, "attachments").Run(check.Input{Files: files}))

	files["ISU-7f3akq"][0].Size = 1 << 20

	require.Len(t, rule(t, "attachments").Run(check.Input{Files: files}), 1)
}

// A ring reached from two directions is one thing to fix. Reported once per
// entrance it would be two, and a reader would spend the second one working out
// that it was the first.
func TestARingIsReportedOnceHoweverManyWaysInThereAre(t *testing.T) {
	t.Parallel()

	in := check.Input{Board: derived(t, map[string]*issue.Issue{
		"ISU-aaaaaa": {ID: "ISU-aaaaaa", BlockedBy: []string{"ISU-bbbbbb"}},
		"ISU-bbbbbb": {ID: "ISU-bbbbbb", BlockedBy: []string{"ISU-aaaaaa"}},
		"ISU-cccccc": {ID: "ISU-cccccc", BlockedBy: []string{"ISU-aaaaaa", "ISU-bbbbbb"}},
	})}

	found := rule(t, "cycles").Run(in)

	require.Len(t, found, 1)
	require.Contains(t, found[0].Message, "ISU-aaaaaa, ISU-bbbbbb")
	require.NotContains(t, found[0].Message, "ISU-cccccc",
		"the frames that reached the ring are not in it")
}

// A claim whose first commit was never looked up is still a claim: what the
// file says is the claim, and the lookup only names who made it.
func TestAClaimWithNoClaimantIsStillAClaim(t *testing.T) {
	t.Parallel()

	item := &model.Item{ID: "ISU-7f3akq", Claims: []model.Claim{
		{Ref: "refs/heads/isu/ISU-7f3akq"},
		{Ref: "refs/heads/isu/ISU-7f3akq-again", Claimant: "bob"},
	}}

	found := rule(t, "claims").Run(check.Input{
		Board: &model.Board{Items: map[string]*model.Item{item.ID: item}},
	})

	require.Len(t, found, 1)
	require.Contains(t, found[0].Message, "isu/ISU-7f3akq (someone)")
	require.Contains(t, found[0].Message, "isu/ISU-7f3akq-again (bob)")
}

// An agent that files an issue has not reassigned anything: there was no owner
// to move.
func TestAnAgentCreatingAnIssueIsNotAReassignment(t *testing.T) {
	t.Parallel()

	found := rule(t, "owner").Run(check.Input{
		Config: config.Config{Agents: []string{"claude"}},
		Branch: &repo.Branch{Commits: []repo.Commit{{
			OID:    "d8cfdb0163",
			Author: gitx.Signature{Name: "claude", Email: "claude@example.invalid"},
			Edits: []repo.Edit{
				{ID: "ISU-7f3akq", Added: true, After: &issue.Issue{Owner: "dmitry"}},
				{ID: "ISU-40b1cc", Removed: true, Before: &issue.Issue{Owner: "dmitry"}},
			},
		}}},
	})

	require.Empty(t, found)
}

// A commit id short enough to be its own abbreviation is left alone, which is
// only ever a fixture's — and a fixture that crashed the renderer would be a
// test suite failing for the wrong reason.
func TestAShortCommitIdIsPrintedAsItIs(t *testing.T) {
	t.Parallel()

	found := rule(t, "owner").Run(check.Input{
		Config: config.Config{Agents: []string{"claude@example.invalid"}},
		Branch: &repo.Branch{Commits: []repo.Commit{{
			OID:    "d8cfdb0",
			Author: gitx.Signature{Name: "claude", Email: "claude@example.invalid"},
			Edits: []repo.Edit{{
				ID:     "ISU-7f3akq",
				Before: &issue.Issue{Owner: "dmitry"},
				After:  &issue.Issue{Owner: "alice"},
			}},
		}}},
	})

	require.Len(t, found, 1)
	require.Contains(t, found[0].Message, "commit d8cfdb0 moves owner")
}

// derived is a board built out of issues rather than out of a repository, for
// the shapes that would take a fixture a page to script.
func derived(t *testing.T, issues map[string]*issue.Issue) *model.Board {
	t.Helper()

	for id, i := range issues {
		i.Folder = id
	}

	return model.Derive(model.Input{Loaded: &repo.Board{
		Trunk:   &repo.Set{Issues: issues},
		Refs:    map[string]*repo.Set{},
		Changed: map[string][]string{},
	}})
}
