package issue_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
)

// frontmatter wraps key/value lines in the delimiters, so that a test case is
// the lines it cares about and nothing else.
func frontmatter(lines ...string) string {
	return "---\n" + strings.Join(lines, "\n") + "\n---\n"
}

// decode parses and decodes, failing the test on either.
func decode(t *testing.T, text string) *issue.Issue {
	t.Helper()

	doc, err := issue.Parse([]byte(text))
	require.NoError(t, err)

	i, err := issue.Decode(doc)
	require.NoError(t, err)

	return i
}

// fields is the sorted list of frontmatter keys an error blames.
func fields(t *testing.T, err error) []string {
	t.Helper()

	var verr *issue.ValidationError
	require.ErrorAs(t, err, &verr, "validation failures are a *ValidationError")

	out := make([]string, 0, len(verr.Problems))
	for _, p := range verr.Problems {
		out = append(out, p.Field)
	}

	return out
}

func TestValidateAcceptsOneIssuePerRowOfTheTypeTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		lines []string
	}{
		{
			name: "a bug carries a repro",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "title: Login retries stop",
				"type: bug", "state: open", "owner: dmitry", "created: 2026-08-24",
				"repro: POST /session five times and watch the fourth 500",
			},
		},
		{
			name: "a story carries an acceptance",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "title: Board renders epics",
				"type: story", "state: open", "owner: dmitry", "created: 2026-08-24",
				"acceptance: children group under their epic",
			},
		},
		{
			name: "a chore carries nothing extra",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "title: Bump the linter",
				"type: chore", "state: open", "owner: dmitry", "created: 2026-08-24",
			},
		},
		{
			name: "a spike carries a question",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "title: How fast is cat-file",
				"type: spike", "state: open", "owner: dmitry", "created: 2026-08-24",
				"question: is one batch process fast enough to skip a cache",
			},
		},
		{
			name: "an epic declares no state",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "title: The board",
				"type: epic", "owner: dmitry", "created: 2026-08-24",
			},
		},
		{
			name: "a dropped issue carries a reason and a resolution",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "title: Support Mercurial",
				"type: chore", "state: dropped", "owner: dmitry", "created: 2026-08-24",
				"reason: the read path is git plumbing top to bottom",
				"resolution: wontfix",
			},
		},
		{
			name: "every optional field at once",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "title: Login retries stop",
				"type: bug", "state: open", "owner: dmitry", "created: 2026-08-24",
				"priority: p0", "parent: ISU-40b1cc",
				"blocked_by: ISU-39ka2p, ISU-2kd8vw",
				"repro: POST /session five times",
			},
		},
		{
			name: "an imported issue keeps its source key as its id",
			lines: []string{
				"schema: 1", "id: PROJ-1234", "title: Imported from Jira",
				"type: bug", "state: open", "owner: dmitry", "created: 2026-07-14",
				"repro: see the attached HAR", "jira_key: PROJ-1234",
			},
		},
		{
			name: "a full RFC 3339 timestamp is a date",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "title: Imported from Jira",
				"type: chore", "state: open", "owner: dmitry",
				"created: 2026-07-14T09:12:00Z",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			i := decode(t, frontmatter(tc.lines...))
			i.Folder = "ISU-7f3akq"
			if i.ID != "ISU-7f3akq" {
				i.Folder = i.ID
			}

			require.NoError(t, i.Validate())
		})
	}
}

func TestValidateRejects(t *testing.T) {
	t.Parallel()

	// base is a valid chore; each case replaces or removes one line of it.
	base := []string{
		"schema: 1", "id: ISU-7f3akq", "title: Bump the linter",
		"type: chore", "state: open", "owner: dmitry", "created: 2026-08-24",
	}

	tests := []struct {
		name   string
		lines  []string
		folder string
		fields []string
		msg    string
	}{
		{
			name:   "an id that is not the folder name",
			lines:  base,
			folder: "ISU-40b1cc",
			fields: []string{"id"},
			msg:    `must equal the folder name, which is "ISU-40b1cc"`,
		},
		{
			name:   "an id with a path separator in it",
			lines:  replace(base, "id: ../etc"),
			fields: []string{"id"},
			msg:    "is not an id",
		},
		{
			name:   "a missing id",
			lines:  remove(base, "id"),
			fields: []string{"id"},
			msg:    "id: required",
		},
		{
			name:   "a missing title",
			lines:  remove(base, "title"),
			fields: []string{"title"},
			msg:    "title: required",
		},
		{
			name:   "a missing created",
			lines:  remove(base, "created"),
			fields: []string{"created"},
			msg:    "created: required",
		},
		{
			name:   "a malformed created",
			lines:  replace(base, "created: last Tuesday"),
			fields: []string{"created"},
			msg:    `"last Tuesday" is not an RFC 3339 date`,
		},
		{
			name:   "a missing owner",
			lines:  remove(base, "owner"),
			fields: []string{"owner"},
			msg:    "owner: required",
		},
		{
			name:   "an unknown type",
			lines:  replace(base, "type: incident"),
			fields: []string{"type"},
			msg:    `"incident" is not a type: expected one of bug, story, chore, spike, epic`,
		},
		{
			name:   "a missing type",
			lines:  remove(base, "type"),
			fields: []string{"type"},
			msg:    "type: required",
		},
		{
			name:   "an unknown state",
			lines:  replace(base, "state: in-progress"),
			fields: []string{"state"},
			msg:    `"in-progress" is not a state: expected one of open, resolved, dropped`,
		},
		{
			name:   "an unknown priority",
			lines:  append(slice(base), "priority: urgent"),
			fields: []string{"priority"},
			msg:    `"urgent" is not a priority`,
		},
		{
			name:   "a parent that is not an id",
			lines:  append(slice(base), "parent: not an id"),
			fields: []string{"parent"},
			msg:    "is not an id",
		},
		{
			name:   "a blocked_by entry that is not an id",
			lines:  append(slice(base), "blocked_by: ISU-40b1cc, ../etc"),
			fields: []string{"blocked_by"},
			msg:    `"../etc" is not an id`,
		},
		{
			name:   "a bug without a repro",
			lines:  replace(base, "type: bug"),
			fields: []string{"repro"},
			msg:    "required on a bug",
		},
		{
			name:   "a story without an acceptance",
			lines:  replace(base, "type: story"),
			fields: []string{"acceptance"},
			msg:    "required on a story",
		},
		{
			name:   "a spike without a question",
			lines:  replace(base, "type: spike"),
			fields: []string{"question"},
			msg:    "required on a spike",
		},
		{
			name:   "an epic declaring a state",
			lines:  replace(base, "type: epic"),
			fields: []string{"state"},
			msg:    "an epic must not declare a state",
		},
		{
			name:   "a non-epic omitting its state",
			lines:  remove(base, "state"),
			fields: []string{"state"},
			msg:    "state: required",
		},
		{
			name:   "dropped without a reason",
			lines:  append(replace(base, "state: dropped"), "resolution: duplicate"),
			fields: []string{"reason"},
			msg:    "required when state is dropped",
		},
		{
			name:   "dropped without a resolution",
			lines:  append(replace(base, "state: dropped"), "reason: nobody asked"),
			fields: []string{"resolution"},
			msg:    "required when state is dropped",
		},
		{
			name: "dropped with a resolution outside the enum",
			lines: append(replace(base, "state: dropped"),
				"reason: nobody asked", "resolution: abandoned"),
			fields: []string{"resolution"},
			msg: `"abandoned" is not a resolution: ` +
				"expected one of wontfix, duplicate, works-as-intended, fixed-elsewhere",
		},
		{
			name: "four problems at once, reported all four and sorted",
			lines: []string{
				"schema: 1", "id: ISU-7f3akq", "type: bug", "state: nonsense",
				"created: whenever",
			},
			fields: []string{"created", "owner", "repro", "state", "title"},
			msg:    "5 problems:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			i := decode(t, frontmatter(tc.lines...))
			i.Folder = tc.folder

			err := i.Validate()
			require.Error(t, err)
			require.Equal(t, tc.fields, fields(t, err))
			require.Contains(t, err.Error(), tc.msg)
		})
	}
}

// The folder check is skipped rather than failed for an issue that is not on
// disk yet, which is what `isu new` builds before it decides where to put it.
func TestValidateSkipsTheFolderCheckWhenThereIsNoFolder(t *testing.T) {
	t.Parallel()

	i := decode(t, frontmatter(
		"schema: 1", "id: ISU-7f3akq", "title: Bump the linter",
		"type: chore", "state: open", "owner: dmitry", "created: 2026-08-24"))

	require.Empty(t, i.Folder)
	require.NoError(t, i.Validate())
}

// Validate takes one issue and nothing else. An epic that nothing points at,
// and a parent naming an issue that does not exist, are both fine here: they
// are questions about a repository, and answering them is M3's job.
func TestValidateNeverLooksAtASecondIssue(t *testing.T) {
	t.Parallel()

	child := decode(t, frontmatter(
		"schema: 1", "id: ISU-7f3akq", "title: A child of nothing",
		"type: chore", "state: open", "owner: dmitry", "created: 2026-08-24",
		"parent: ISU-nowhere"))
	require.NoError(t, child.Validate())

	epic := decode(t, frontmatter(
		"schema: 1", "id: ISU-40b1cc", "title: An epic with no children",
		"type: epic", "owner: dmitry", "created: 2026-08-24"))
	require.NoError(t, epic.Validate())
}

func TestValidateReportsStableSortedOutput(t *testing.T) {
	t.Parallel()

	i := decode(t, frontmatter("schema: 1", "type: bug", "state: dropped"))

	err := i.Validate()
	require.Error(t, err)

	first := err.Error()
	for range 20 {
		require.Equal(t, first, i.Validate().Error(), "the message must not move between runs")
	}

	require.Equal(t, strings.Join([]string{
		"7 problems:",
		"  created: required: an RFC 3339 date, such as 2026-08-24",
		"  id: required",
		"  owner: required: name the human answerable for this issue",
		"  reason: required when state is dropped",
		`  repro: required on a bug: say how to reproduce it`,
		"  resolution: required when state is dropped: " +
			"expected one of wontfix, duplicate, works-as-intended, fixed-elsewhere",
		"  title: required",
	}, "\n"), first)
}

func TestValidationErrorRendersOneProblemOnOneLine(t *testing.T) {
	t.Parallel()

	i := decode(t, frontmatter(
		"schema: 1", "id: ISU-7f3akq", "type: chore", "state: open",
		"owner: dmitry", "created: 2026-08-24"))

	require.EqualError(t, i.Validate(), "1 problem: title: required")
}

func TestDecode(t *testing.T) {
	t.Parallel()

	i := decode(t, "---\n"+strings.Join([]string{
		"schema: 1",
		"id: ISU-7f3akq",
		"title: Login retries stop after the third attempt",
		"type: bug",
		"state: dropped",
		"owner: dmitry",
		"created: 2026-08-24",
		"priority: p1",
		"parent: ISU-40b1cc",
		"blocked_by: ISU-39ka2p ,, ISU-2kd8vw ",
		"repro: POST /session five times",
		"acceptance: n/a",
		"question: n/a",
		"reason: it is a duplicate of ISU-40b1cc",
		"resolution: duplicate",
		"jira_key: PROJ-1234",
	}, "\n")+"\n---\nThe body.\n")

	require.Equal(t, 1, i.Schema)
	require.Equal(t, "ISU-7f3akq", i.ID)
	require.Equal(t, "Login retries stop after the third attempt", i.Title)
	require.Equal(t, issue.TypeBug, i.Type)
	require.Equal(t, issue.StateDropped, i.State)
	require.Equal(t, "dmitry", i.Owner)
	require.Equal(t, time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), i.Created)
	require.Equal(t, issue.PriorityP1, i.Priority)
	require.Equal(t, "ISU-40b1cc", i.Parent)
	require.Equal(t, []string{"ISU-39ka2p", "ISU-2kd8vw"}, i.BlockedBy)
	require.Equal(t, "POST /session five times", i.Repro)
	require.Equal(t, "n/a", i.Acceptance)
	require.Equal(t, "n/a", i.Question)
	require.Equal(t, "it is a duplicate of ISU-40b1cc", i.Reason)
	require.Equal(t, issue.ResolutionDuplicate, i.Resolution)
	require.Equal(t, "The body.\n", i.Body)
}

// The default is applied where it is read, not where the file is parsed, so
// that writing the issue back does not add a line the author never wrote.
func TestEffectivePriority(t *testing.T) {
	t.Parallel()

	i := decode(t, frontmatter("schema: 1", "id: ISU-7f3akq"))
	require.Empty(t, i.Priority)
	require.Equal(t, issue.PriorityP2, i.EffectivePriority())

	i.Priority = issue.PriorityP0
	require.Equal(t, issue.PriorityP0, i.EffectivePriority())
}

func TestEnums(t *testing.T) {
	t.Parallel()

	for _, v := range issue.Types {
		require.True(t, v.Valid(), "%q", v)
	}
	require.False(t, issue.Type("incident").Valid())

	for _, v := range issue.States {
		require.True(t, v.Valid(), "%q", v)
	}
	require.False(t, issue.State("in-progress").Valid())

	for _, v := range issue.Priorities {
		require.True(t, v.Valid(), "%q", v)
	}
	require.False(t, issue.Priority("urgent").Valid())

	for _, v := range issue.Resolutions {
		require.True(t, v.Valid(), "%q", v)
	}
	require.False(t, issue.Resolution("abandoned").Valid())

	require.False(t, issue.StateOpen.Terminal())
	require.True(t, issue.StateResolved.Terminal())
	require.True(t, issue.StateDropped.Terminal())
}

func TestValidID(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"ISU-7f3akq", "PROJ-1234", "a", "A_b.c-1"} {
		require.True(t, issue.ValidID(id), "%q", id)
	}

	for _, id := range []string{"", ".", "..", "a/b", "a b", "a:b", "iss\nue", "ISU-✅"} {
		require.False(t, issue.ValidID(id), "%q", id)
	}
}

func TestProblemStringWithoutAField(t *testing.T) {
	t.Parallel()

	require.Equal(t, "something is wrong",
		issue.Problem{Message: "something is wrong"}.String())
	require.Equal(t, "title: required",
		issue.Problem{Field: "title", Message: "required"}.String())
}

// Three things Validate reports that a decoded file cannot produce: the
// document reader refuses a schema it does not know before Validate ever runs,
// and the frontmatter parser cannot hand back a title with a line break in it.
// An Issue built in memory can be all three, and `isu new` builds one.
func TestValidateAnIssueBuiltInMemory(t *testing.T) {
	t.Parallel()

	i := &issue.Issue{
		Schema:    2,
		ID:        "ISU-7f3akq",
		Title:     "Two\nlines",
		Type:      issue.TypeChore,
		State:     issue.StateOpen,
		Owner:     "dmitry",
		Created:   mustDate(t, "2026-08-24"),
		BlockedBy: []string{"../a", "../b"},
	}

	require.EqualError(t, i.Validate(), strings.Join([]string{
		"4 problems:",
		`  blocked_by: "../a" is not an id`,
		`  blocked_by: "../b" is not an id`,
		"  schema: must be 1, found 2",
		"  title: must be one line",
	}, "\n"), "two problems about one key sort by message, and both are reported")
}

// slice copies a case's base lines, because append on a shared backing array
// makes one table row change another.
func slice(lines []string) []string {
	out := make([]string, len(lines))
	copy(out, lines)

	return out
}

// replace swaps the line whose key matches the one in want.
func replace(lines []string, want string) []string {
	key := strings.SplitN(want, ":", 2)[0]

	out := slice(lines)
	for i, line := range out {
		if strings.SplitN(line, ":", 2)[0] == key {
			out[i] = want

			return out
		}
	}

	return append(out, want)
}

// remove drops the line with the given key.
func remove(lines []string, key string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.SplitN(line, ":", 2)[0] != key {
			out = append(out, line)
		}
	}

	return out
}
