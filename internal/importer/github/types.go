package github

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// The shapes GitHub's REST list returns, plus the four keys `gh issue list
// --json` spells differently and the four this importer's own dump adds.
//
// They are exported because the dump is a file format rather than an internal
// convenience: `--fetch-only` writes one, a person reads it, and a person may
// well assemble one. The aliases are normalise's business rather than a second
// parser's — a field that exists under two names is not a reason for two
// readers.

// Actor is whoever GitHub says did something: an author, an assignee, a
// milestone's creator.
type Actor struct {
	Login string `json:"login"`
}

// Label is one label on an issue. Only its name is read: a colour is not a
// type, and nothing else here is either.
type Label struct {
	Name string `json:"name"`
}

// Named is anything GitHub identifies by a name — an issue type, an issue
// field — where the name is the whole of what this importer needs.
type Named struct {
	Name string `json:"name"`
}

// Milestone is a named container whose progress is a fold over the issues in
// it, which is exactly what an isu epic is. An issue belongs to at most one,
// so it lands in the single `parent` field with no collapse to design.
type Milestone struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	DueOn       *time.Time `json:"due_on"`
	Creator     *Actor     `json:"creator"`
}

// Dependency is one issue this one is blocked by — GitHub's issue
// dependencies, August 2025, which is isu's `blocked_by` under its own name.
// Repository is set only when the blocker is in another one.
type Dependency struct {
	Number     int    `json:"number"`
	Repository string `json:"repository"`
}

// ClosingRef is a pull request GitHub says closed an issue. It is the
// strongest evidence tier there is and it arrives with the issue, rather than
// through an integration somebody had to install.
type ClosingRef struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// Comment is one comment on an issue. Each becomes a file under `comments/`,
// named for its day, its author and its place in that day.
type Comment struct {
	User      *Actor    `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	Body      string    `json:"body"`
}

// FieldValue is one issue field value — the structured custom metadata that
// reached general availability in July 2026, and precisely the two hundred
// custom fields PLAN.md M7-S1 refuses to let into frontmatter.
type FieldValue struct {
	Name  string `json:"name"`
	Field *Named `json:"field"`
	Value any    `json:"value"`
}

func (f FieldValue) name() string {
	if f.Field != nil && f.Field.Name != "" {
		return f.Field.Name
	}

	return f.Name
}

func (f FieldValue) value() any { return f.Value }

// Issue is one row of the REST list, and the unit this importer's dump is a
// list of.
type Issue struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Body        string     `json:"body,omitempty"`
	State       string     `json:"state"`
	StateReason string     `json:"state_reason,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	HTMLURL     string     `json:"html_url,omitempty"`

	User      *Actor     `json:"user,omitempty"`
	Assignees []Actor    `json:"assignees,omitempty"`
	Labels    []Label    `json:"labels,omitempty"`
	Milestone *Milestone `json:"milestone,omitempty"`
	Type      *Named     `json:"type,omitempty"`

	// PullRequest is how the REST list says a row is a pull request rather
	// than an issue. Its contents are never read; its presence is the whole
	// signal.
	PullRequest json.RawMessage `json:"pull_request,omitempty"`

	// Comments is an integer in the REST list and an array in a dump that
	// carries them, so it is held raw and decoded only when it is the second.
	Comments json.RawMessage `json:"comments,omitempty"`

	// ParentIssueURL is the sub-issue link the REST list carries itself, which
	// is what makes the hierarchy free: no request per issue, and no dump
	// extension needed to have it.
	ParentIssueURL string `json:"parent_issue_url,omitempty"`

	// SubIssues, BlockedBy, Fields and ClosedByCommit are what this importer's
	// own dump adds to the REST row. SubIssues is the other direction of the
	// hierarchy, for a dump assembled from the sub-issues endpoint; the rest
	// have no place in the list at all — GitHub answers for dependencies and
	// field values per issue — so a repository that uses none of them imports
	// with none of them.
	SubIssues      []int        `json:"sub_issues,omitempty"`
	BlockedBy      []Dependency `json:"blocked_by,omitempty"`
	Fields         []FieldValue `json:"issue_field_values,omitempty"`
	ClosedBy       []ClosingRef `json:"closedByPullRequestsReferences,omitempty"`
	ClosedByCommit string       `json:"closed_by_commit,omitempty"`

	// The `gh issue list --json` spellings. They are pointers and omitempty so
	// that a dump this importer writes carries the REST spelling and nothing
	// else — a file with every key under two names is a file nobody can read.
	StateReasonAlias string     `json:"stateReason,omitempty"`
	CreatedAtAlias   *time.Time `json:"createdAt,omitempty"`
	URLAlias         string     `json:"url,omitempty"`
	AuthorAlias      *Actor     `json:"author,omitempty"`
	TypeAlias        *Named     `json:"issueType,omitempty"`

	// comments is the decoded list, and is filled by normalise.
	comments []Comment
}

// isPullRequest is the first question this importer asks of a row, and skipping
// it is the classic bug in every importer that skips it.
//
// A dump this importer wrote carries the key back as a JSON null on every
// ordinary issue, so its presence alone is not the test — `null` is what
// "there is no pull request here" round-trips as.
func (in *Issue) isPullRequest() bool {
	return len(in.PullRequest) > 0 && !bytes.Equal(in.PullRequest, []byte("null"))
}

// state is `open` or `closed`, whichever case the source wrote it in.
func (in *Issue) state() string { return strings.ToLower(in.State) }

// normalise folds the `gh` spellings onto the REST ones and lower-cases the
// two enumerations, which `gh` shouts and REST does not.
func (in *Issue) normalise() {
	if in.StateReason == "" {
		in.StateReason = in.StateReasonAlias
	}

	in.StateReason = strings.ToLower(in.StateReason)

	if in.CreatedAt.IsZero() && in.CreatedAtAlias != nil {
		in.CreatedAt = *in.CreatedAtAlias
	}

	if in.HTMLURL == "" {
		in.HTMLURL = in.URLAlias
	}

	if in.User == nil {
		in.User = in.AuthorAlias
	}

	if in.Type == nil {
		in.Type = in.TypeAlias
	}

	in.decodeComments()
}

// decodeComments reads the comments when there are any to read.
//
// The REST list puts a count under this key and a dump carrying the comments
// themselves puts an array there, so the two are told apart by what the value
// is rather than by which producer wrote it. Anything that is neither is
// nothing: a count is not a comment, and refusing the whole import over it
// would refuse every REST dump.
func (in *Issue) decodeComments() {
	trimmed := strings.TrimLeft(string(in.Comments), " \t\r\n")
	if !strings.HasPrefix(trimmed, "[") {
		return
	}

	var list []Comment
	if err := json.Unmarshal(in.Comments, &list); err == nil {
		in.comments = list
	}
}
