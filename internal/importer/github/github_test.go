package github_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/importer/github"
	"github.com/dgorshkov/isu/internal/issue"
)

// Everything here reads a recorded dump and never the live API. An importer
// tested against a service that changes underneath it is one whose test suite
// fails for reasons nobody controls, and PLAN.md M7-S5 says so in as many
// words: "against a recorded transcript, never the live API".

func dump(t *testing.T, name string) []byte {
	t.Helper()

	body, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)

	return body
}

// load reads a fixture through the whole source.
func load(t *testing.T, name string, opts github.Options) *importer.Batch {
	t.Helper()

	opts.Dump = dump(t, name)

	source, err := github.New(opts)
	require.NoError(t, err)

	batch, err := source.Load(t.Context())
	require.NoError(t, err)

	return batch
}

// item is one issue out of a batch, by key.
func item(t *testing.T, b *importer.Batch, key string) importer.Item {
	t.Helper()

	for _, one := range b.Items {
		if one.Key == key {
			return one
		}
	}

	t.Fatalf("no %s in %v", key, keys(b))

	return importer.Item{}
}

func keys(b *importer.Batch) []string {
	out := make([]string, 0, len(b.Items))
	for _, one := range b.Items {
		out = append(out, one.Key)
	}

	return out
}

// planned maps a fixture the way `isu import` does.
func planned(t *testing.T, b *importer.Batch, opts importer.Options) *importer.Plan {
	t.Helper()

	if opts.Prefix == "" {
		opts.Prefix = "ISU"
	}

	m, err := importer.Keys(b, opts.Prefix)
	require.NoError(t, err)

	plan, err := importer.Map(b, m, opts)
	require.NoError(t, err)

	return plan
}

func folder(t *testing.T, plan *importer.Plan, id string) importer.Folder {
	t.Helper()

	for _, f := range plan.Folders {
		if f.ID == id {
			return f
		}
	}

	t.Fatalf("no folder %s", id)

	return importer.Folder{}
}

func body(t *testing.T, f importer.Folder, name string) string {
	t.Helper()

	for _, one := range f.Files {
		if one.Name == name {
			return string(one.Body)
		}
	}

	t.Fatalf("%s has no %s", f.ID, name)

	return ""
}

// PLAN.md M7-S4: "assert pull requests in the list are not imported". The REST
// list returns both and they are told apart by the pull_request key; skipping
// this is the classic bug in every importer that skips it.
func TestAPullRequestIsNotAnIssue(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})

	require.NotContains(t, keys(b), "#7")

	var why string

	for _, skip := range b.Skipped {
		if skip.Ref == "#7" {
			why = skip.Why
		}
	}

	require.Contains(t, why, "pull request")
}

func TestEveryStateReasonIncludingItsAbsence(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})

	for _, tt := range []struct {
		key        string
		state      issue.State
		resolution issue.Resolution
	}{
		{"#1", issue.StateOpen, ""},
		{"#2", issue.StateResolved, ""},
		{"#3", issue.StateResolved, ""},
		{"#4", issue.StateDropped, issue.ResolutionWontfix},
		{"#5", issue.StateDropped, issue.ResolutionDuplicate},
		{"#6", issue.StateOpen, ""},
	} {
		one := item(t, b, tt.key)
		require.Equalf(t, tt.state, one.State, "%s", tt.key)
		require.Equalf(t, tt.resolution, one.Resolution, "%s", tt.key)

		if tt.state == issue.StateDropped {
			require.NotEmptyf(t, one.Reason, "%s is dropped and says nothing about why", tt.key)
		}
	}
}

// PLAN.md M7-S4: "assert every dropped issue has a resolution from the enum and
// a reason."
func TestEveryDroppedIssueCarriesAResolutionFromTheEnumAndAReason(t *testing.T) {
	t.Parallel()

	plan := planned(t, load(t, "dump.json", github.Options{}), importer.Options{})

	dropped := 0

	for _, f := range plan.Folders {
		if f.Issue.State != issue.StateDropped {
			continue
		}

		dropped++

		require.Truef(t, f.Issue.Resolution.Valid(), "%s: %q", f.ID, f.Issue.Resolution)
		require.NotEmpty(t, f.Issue.Reason)
	}

	require.Equal(t, 2, dropped)
}

// PLAN.md M7-S4: "assert imported milestones carry type: epic and no state:".
func TestAMilestoneBecomesAnEpicAndAnEpicDeclaresNoState(t *testing.T) {
	t.Parallel()

	plan := planned(t, load(t, "dump.json", github.Options{}), importer.Options{})

	epic := folder(t, plan, "ISU-M1")
	require.Equal(t, issue.TypeEpic, epic.Issue.Type)
	require.Empty(t, epic.Issue.State)
	require.Equal(t, "Make login reliable", epic.Issue.Title)
	require.Equal(t, "dmitry", epic.Issue.Owner, "the milestone's creator")
	require.Equal(t, "Everything about signing in.\n", epic.Issue.Body)

	source := body(t, epic, importer.SourceFileName)
	require.Contains(t, source, "milestone_state: open")
	require.Contains(t, source, "milestone_due_on: \"2026-06-30\"")

	require.Equal(t, "ISU-M1", folder(t, plan, "ISU-1").Issue.Parent)
	require.Equal(t, "ISU-M1", folder(t, plan, "ISU-2").Issue.Parent)
}

// PLAN.md M7-S4: "a milestone with no imported children is not written at all",
// and "assert an issue whose milestone was skipped emits no parent".
func TestAMilestoneWithNoImportedChildrenIsNotWritten(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})

	require.NotContains(t, keys(b), "#M2",
		"milestone 2 belongs to a pull request and nothing else, and M5-S2 makes an "+
			"empty epic a check failure")

	// --state open empties every milestone whose issues are all closed, which
	// is the same rule arriving from the other direction.
	open := load(t, "dump.json", github.Options{State: github.StateOpen})
	require.Contains(t, keys(open), "#M1", "issue #1 is open and is in milestone 1")

	closed := load(t, "dump.json", github.Options{State: github.StateClosed})
	require.Contains(t, keys(closed), "#M1")
	require.NotContains(t, keys(closed), "#1")
}

// PLAN.md M7-S4: "assert an out-of-import blocker leaves blocked_by absent
// rather than dangling."
func TestADependencyOutsideTheImportIsRecordedAndNotWritten(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})

	require.Equal(t, []string{"#3", "#999", "other/repo#5"}, item(t, b, "#1").BlockedBy)

	plan := planned(t, b, importer.Options{})
	one := folder(t, plan, "ISU-1")

	require.Equal(t, []string{"ISU-3"}, one.Issue.BlockedBy)
	require.NotContains(t, body(t, one, issue.ReadmeName), "999")

	source := body(t, one, importer.SourceFileName)
	require.Contains(t, source, "#999")
	require.Contains(t, source, "other/repo#5")
}

// PLAN.md M7-S4: "a four-level sub-issue tree". isu has one parent, the
// milestone is spending it, so the hierarchy is recorded whole and modelled not
// at all.
func TestASubIssueTreeIsRecordedWholeAndNeverBecomesAParent(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})
	plan := planned(t, b, importer.Options{})

	root := folder(t, plan, "ISU-10")
	require.Empty(t, root.Issue.Parent, "a sub-issue tree is not an epic")

	source := body(t, root, importer.SourceFileName)
	require.Contains(t, source, "sub_issues:")
	require.Contains(t, source, "#11")
	require.Contains(t, source, "#12")
	require.Contains(t, source, "#13")
	require.Contains(t, source, "#14")

	child := folder(t, plan, "ISU-13")
	require.Empty(t, child.Issue.Parent)
	require.Contains(t, body(t, child, importer.SourceFileName), "sub_issue_of: \"#12\"")

	require.NotContains(t, body(t, folder(t, plan, "ISU-11"), importer.SourceFileName),
		"\nsub_issues:", "only the root of a tree carries the whole of it")

	require.Contains(t, b.Notes[0], "4 levels deep")
	require.Contains(t, b.Notes[0], "5 issues across")
}

func TestTheTypeIsTheOrganisationsThenTheMapThenAChore(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})

	require.Equal(t, issue.TypeBug, item(t, b, "#1").Type, "GitHub's own Bug")
	require.Equal(t, issue.TypeChore, item(t, b, "#6").Type, "GitHub's own Task")
	require.Equal(t, issue.TypeChore, item(t, b, "#2").Type, "no type, and no map")
	require.Equal(t, issue.TypeChore, item(t, b, "#8").Type, "a type nobody mapped")

	require.Equal(t, []string{"Chore of ours", "enhancement"}, b.Unplaced,
		"the dry run lists what it could not place, and an issue GitHub's own type "+
			"placed never puts its labels on that list")

	mapped := load(t, "dump.json", github.Options{
		TypeMap: map[string]issue.Type{"enhancement": issue.TypeStory, "chore of ours": issue.TypeSpike},
	})

	require.Equal(t, issue.TypeStory, item(t, mapped, "#2").Type)
	require.Equal(t, issue.TypeSpike, item(t, mapped, "#8").Type)
}

func TestTheAuthorIsRecordedAndIsDeliberatelyNotTheOwner(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{})
	one := item(t, b, "#1")

	require.Equal(t, "dmitry", one.Owner, "the first assignee")
	require.Equal(t, "support", one.Extra["author"],
		"the person who filed a bug is usually not the person answerable for it")
	require.Equal(t, []any{"dmitry", "alice"}, one.Extra["assignees"])
	require.Equal(t, []string{"login", "regression"}, one.Extra["labels"])
	require.Equal(t, "Bug", one.Extra["issue_type"])
	require.Equal(t, "Make login reliable", one.Extra["milestone"])

	require.Empty(t, item(t, b, "#8").Owner)
	require.Equal(t, 1, planned(t, b, importer.Options{}).Report.Unowned)
}

// PLAN.md M7: "a repository with none of them still imports, and the dry run
// says which it found."
func TestARepositoryThatUsesNoneOfItStillImports(t *testing.T) {
	t.Parallel()

	b := load(t, "plain.json", github.Options{})

	require.Equal(t, "plain/repo", b.Repository,
		"a bare array still knows what it is a dump of, from an issue's own URL")
	require.Empty(t, b.Found)

	plan := planned(t, b, importer.Options{})
	require.Len(t, plan.Folders, 1)
	require.NoError(t, plan.Folders[0].Issue.Validate())

	rich := load(t, "dump.json", github.Options{})
	require.Subset(t, rich.Found,
		[]string{"dependencies", "issue types", "milestones", "sub-issues"})
}

func TestADumpWrittenByGHIsReadableToo(t *testing.T) {
	t.Parallel()

	b := load(t, "gh.json", github.Options{})
	one := item(t, b, "#42")

	require.Equal(t, "acme/widgets", b.Repository)
	require.Equal(t, issue.StateDropped, one.State)
	require.Equal(t, issue.ResolutionWontfix, one.Resolution)
	require.Equal(t, issue.TypeStory, one.Type, "gh spells GitHub's Feature under issueType")
	require.Equal(t, "support", one.Extra["author"])
	require.Equal(t, "https://github.com/acme/widgets/issues/42", one.URL)
	require.Equal(t, 2026, one.Created.Year())
}

func TestTheKeysThisSourceReadsOutOfProse(t *testing.T) {
	t.Parallel()

	source, err := github.New(github.Options{Dump: []byte("[]")})
	require.NoError(t, err)

	require.Equal(t, github.Name, source.Name())
	require.NotEmpty(t, source.Describe())

	require.Equal(t, []string{"#456", "#1"},
		source.Keys("Merge pull request #456 from alice/fix-1 (closes #1, and #456 again)"))
	require.Nil(t, source.Keys("nothing here, and a # on its own"))
}

func TestAnOptionThatCannotWorkIsRefused(t *testing.T) {
	t.Parallel()

	_, err := github.New(github.Options{Dump: []byte("[]"), State: "half-open"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--state is one of")

	_, err = github.New(github.Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "a saved dump")

	_, err = github.New(github.Options{Client: &github.Client{}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "owner/repo")
}

func TestADumpThatIsNotOneIsRefused(t *testing.T) {
	t.Parallel()

	for _, body := range []string{"[not json", "{not json"} {
		source, err := github.New(github.Options{Dump: []byte(body)})
		require.NoError(t, err)

		_, err = source.Load(t.Context())
		require.Error(t, err)
		require.Contains(t, err.Error(), "reading the dump")
	}
}

func TestTheRepositoryOnTheCommandLineWinsOverTheDumps(t *testing.T) {
	t.Parallel()

	b := load(t, "dump.json", github.Options{Repository: "fork/widgets"})
	require.Equal(t, "fork/widgets", b.Repository)
	require.Equal(t, "fork/widgets#1", item(t, b, "#1").Ref)
}

// PLAN.md M7-S4: "the imported tree passes isu check with zero failures" — the
// half of it this package can assert on its own is that every issue it produces
// is a valid one.
func TestEveryIssueTheImporterProducesValidates(t *testing.T) {
	t.Parallel()

	plan := planned(t, load(t, "dump.json", github.Options{}), importer.Options{Owner: "support"})

	require.NotEmpty(t, plan.Folders)

	for _, f := range plan.Folders {
		require.NoErrorf(t, f.Issue.Validate(), "%s", f.ID)
	}

	require.Zero(t, plan.Report.Unowned)
}

func TestATreeDeeperThanGitHubAllowsIsBoundedRatherThanFollowed(t *testing.T) {
	t.Parallel()

	// GitHub's own limit is eight levels; a dump somebody edited has no limit
	// at all, so the walk carries its own bound rather than trusting the shape.
	b := load(t, "deep.json", github.Options{})

	require.Contains(t, b.Notes[0], "9 levels deep",
		"the walk stops itself one level past GitHub's own limit rather than "+
			"following a dump somebody edited")
	require.Contains(t, b.Notes[0], "10 issues across")

	plan := planned(t, b, importer.Options{})
	source := body(t, folder(t, plan, "ISU-1"), importer.SourceFileName)

	require.Contains(t, source, "#9")
	require.NotContains(t, source, "#10", "the tree is recorded to the depth GitHub allows")
}

func TestADumpThatNeverSaysWhatItIsADumpOf(t *testing.T) {
	t.Parallel()

	b := load(t, "nameless.json", github.Options{})

	require.Empty(t, b.Repository)
	require.Equal(t, "#5", item(t, b, "#5").Ref,
		"a ref with no repository in front of it is still the key a human writes")
}

func TestAMilestoneWithNoDateOfItsOwnIsDatedByItsChildren(t *testing.T) {
	t.Parallel()

	plan := planned(t, load(t, "gh.json", github.Options{}), importer.Options{Owner: "dmitry"})

	epic := folder(t, plan, "ISU-M9")
	require.Equal(t, issue.TypeEpic, epic.Issue.Type)
	require.Equal(t, "2026-04-04", epic.Issue.Created.UTC().Format("2006-01-02"),
		"created is required by the schema, and a milestone read from gh carries none")
	require.Equal(t, "dmitry", epic.Issue.Owner, "the milestone has no creator, so --owner")
}
