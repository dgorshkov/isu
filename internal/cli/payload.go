package cli

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// The types in this file are isu's JSON output, and they are a contract rather
// than a rendering. Half the users of this tool are agents; an agent that has to
// re-learn the shape of a board every release is an agent nobody automates
// against.
//
// docs/json.md documents them, and TestJSONDocumentsEveryField fails the build
// when a field is added here and not written down there — so the document
// cannot drift from the code by more than one commit that nobody ran the tests
// on.
//
// Every field is always present. Omitting empty ones would make a consumer
// distinguish "absent" from "false" for no gain, and `jq .claims[0]` on a
// missing key is a different failure from one on an empty list.

// Failure is what any command prints on stderr when --json is set and it could
// not do what it was asked. It is the only thing printed: an agent parsing
// stderr should not have to find one JSON object in a page of usage text.
type Failure struct {
	Error string `json:"error"`
}

// Issue is one issue as --json renders it: the file's own fields, and what the
// repository derives about them.
type Issue struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	State     string   `json:"state"`
	Status    string   `json:"status"`
	Owner     string   `json:"owner"`
	Created   string   `json:"created"`
	Priority  string   `json:"priority"`
	Parent    string   `json:"parent"`
	BlockedBy []string `json:"blocked_by"`
	// The fields a type requires, which are what somebody picking this up has
	// to read before they can start. They are one line each and they are on
	// every issue rather than only on `isu show`, because `isu ready --json |
	// head -1` is meant to be the whole briefing.
	Repro      string   `json:"repro"`
	Acceptance string   `json:"acceptance"`
	Question   string   `json:"question"`
	Reason     string   `json:"reason"`
	Resolution string   `json:"resolution"`
	OnTrunk    bool     `json:"on_trunk"`
	Reopened   bool     `json:"reopened"`
	Contended  bool     `json:"contended"`
	Stale      bool     `json:"stale"`
	Claims     []Claim  `json:"claims"`
	Elsewhere  []string `json:"elsewhere"`
	Epic       *Epic    `json:"epic"`
	Broken     *Broken  `json:"broken"`
}

// Claim is one branch claiming one issue.
type Claim struct {
	Ref        string `json:"ref"`
	Claimant   string `json:"claimant"`
	Email      string `json:"email"`
	Commit     string `json:"commit"`
	When       string `json:"when"`
	AgeSeconds int64  `json:"age_seconds"`
	Stale      bool   `json:"stale"`
}

// Epic is what an epic's children add up to. It is null on everything that is
// not one.
type Epic struct {
	Children []string `json:"children"`
	Cycle    bool     `json:"cycle"`
}

// Broken is why trunk's copy of an issue could not be read. It is null when it
// could, which is almost always — and when it is not, the issue is still on the
// board, because a half-written issue must not blind the whole board.
type Broken struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Freshness is how old the newest remote ref is.
//
// Every derived status in this product is a statement about refs, so a board
// computed from a week-old fetch is a board about last week. Two engineers
// looking at the same repository with different fetch ages see different
// contention, and the output says so rather than letting them argue about it.
type Freshness struct {
	// Remote says whether there are remote-tracking refs at all. In a
	// repository with none there is nothing to be behind on.
	Remote bool `json:"remote"`
	// Newest is the date of the newest remote-tracking ref, RFC 3339, or empty
	// when there are none.
	Newest string `json:"newest"`
	// AgeSeconds is how old that is, and zero when there are none.
	AgeSeconds int64 `json:"age_seconds"`
	// Warn says the age is past the repository's fetch_warn_hours.
	Warn bool `json:"warn"`
}

// BoardPayload is `isu board`.
type BoardPayload struct {
	Trunk     string    `json:"trunk"`
	Refs      []string  `json:"refs"`
	Freshness Freshness `json:"freshness"`
	Groups    []Group   `json:"groups"`
}

// Group is one status's issues, in the precedence order of PLAN.md's table.
type Group struct {
	Status string  `json:"status"`
	Issues []Issue `json:"issues"`
}

// ShowPayload is `isu show`: one issue, and everything that lives beside it.
type ShowPayload struct {
	Issue Issue `json:"issue"`
	// Body is the markdown below the frontmatter, verbatim. v1.0.0 prints it
	// rather than rendering it — glamour arrives with the TUI in M6.
	Body string `json:"body"`
	// Attachments are the files beside the README, by name. Their contents are
	// arbitrary bytes and are not read.
	Attachments []string `json:"attachments"`
	Comments    []Text   `json:"comments"`
	// Children are the issues naming this one as their parent, which is empty
	// on everything that is not an epic.
	Children []Issue `json:"children"`
	// Epic is the issue this one belongs to, resolved to its title, or null.
	Parent    *Link     `json:"parent"`
	Blockers  []Link    `json:"blockers"`
	Freshness Freshness `json:"freshness"`
}

// Text is one comment file: what it is called, and what it says.
type Text struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

// Link is another issue named by this one, with enough of it to render.
type Link struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	// Known says the id resolves to an issue in this repository. A blocker or a
	// parent that names nothing is M5-S2's to report and is not dropped here.
	Known bool `json:"known"`
}

// Write is what every command that changes the repository reports.
//
// One shape rather than eight: a caller that has run `isu claim` and `isu
// resolve` should not have to learn two answers to "what did you just do".
// Fields a command has nothing to say about are empty — `isu init` names no
// issue, `isu new --no-branch` writes no commit.
type Write struct {
	// ID is the issue that changed.
	ID string `json:"id"`
	// Branch is the branch written to, or empty when the command wrote into the
	// working tree instead.
	Branch string `json:"branch"`
	// Commit is the commit written, or empty when the command left the change
	// uncommitted.
	Commit string `json:"commit"`
	// Paths are what changed, relative to the repository root.
	Paths []string `json:"paths"`
	// Pushed says the branch reached the remote. Only `claim` and `unclaim`
	// push, and `triage --push`.
	Pushed bool `json:"pushed"`
}

// emit writes one JSON value and the newline that ends its line.
func emit(w io.Writer, value any) error {
	return json.NewEncoder(w).Encode(value)
}

// asIssue renders one derived item.
//
// The zero-length slices are deliberate: a JSON array that is sometimes null
// makes every consumer write the same defensive branch, and this is a contract.
func asIssue(item *model.Item, now time.Time) Issue {
	out := Issue{
		ID:        item.ID,
		Status:    string(item.Status),
		OnTrunk:   item.OnTrunk,
		Reopened:  item.Reopened,
		Contended: item.Contended(),
		Stale:     item.Stale(),
		BlockedBy: []string{},
		Claims:    []Claim{},
		Elsewhere: []string{},
	}

	if item.Issue != nil {
		out.Title = item.Issue.Title
		out.Type = string(item.Issue.Type)
		out.State = string(item.Issue.State)
		out.Owner = item.Issue.Owner
		out.Priority = string(item.Issue.EffectivePriority())
		out.Parent = item.Issue.Parent

		if !item.Issue.Created.IsZero() {
			out.Created = item.Issue.Created.Format(time.DateOnly)
		}

		out.Repro = item.Issue.Repro
		out.Acceptance = item.Issue.Acceptance
		out.Question = item.Issue.Question
		out.Reason = item.Issue.Reason
		out.Resolution = string(item.Issue.Resolution)

		out.BlockedBy = append(out.BlockedBy, item.Issue.BlockedBy...)
	}

	out.Elsewhere = append(out.Elsewhere, item.Elsewhere...)

	for _, claim := range item.Claims {
		out.Claims = append(out.Claims, asClaim(claim, now))
	}

	if item.Epic != nil {
		out.Epic = &Epic{Children: append([]string{}, item.Epic.Children...), Cycle: item.Epic.Cycle}
	}

	if item.Broken != nil {
		out.Broken = &Broken{Path: item.Broken.Path, Error: item.Broken.Err.Error()}
	}

	return out
}

func asClaim(claim model.Claim, now time.Time) Claim {
	out := Claim{
		Ref:      claim.Ref,
		Claimant: claim.Claimant,
		Email:    claim.Email,
		Commit:   claim.Commit,
		Stale:    claim.Stale,
	}

	if !claim.When.IsZero() {
		out.When = claim.When.UTC().Format(time.RFC3339)
		out.AgeSeconds = int64(now.Sub(claim.When).Seconds())
	}

	return out
}

// asLink resolves an id another issue named, so that a renderer never has to
// hold the whole board to say what a blocker is called.
func asLink(board *model.Board, id string) Link {
	link := Link{ID: id}

	item, ok := board.Get(id)
	if !ok {
		return link
	}

	link.Known = true
	link.Status = string(item.Status)

	if item.Issue != nil {
		link.Title = item.Issue.Title
	}

	return link
}

// freshness reads how old the newest remote-tracking ref is.
//
// It is one `for-each-ref` over refs/remotes/, which is where a fetch puts what
// it found. A repository with no remote refs has nothing to be behind on and
// says so rather than reporting an age of fifty-six years.
func (s *session) freshness(refs []gitx.Ref, now time.Time) Freshness {
	var newest time.Time

	for _, ref := range refs {
		if ref.Created.After(newest) {
			newest = ref.Created
		}
	}

	if newest.IsZero() {
		return Freshness{}
	}

	age := now.Sub(newest)

	return Freshness{
		Remote:     true,
		Newest:     newest.UTC().Format(time.RFC3339),
		AgeSeconds: int64(age.Seconds()),
		Warn:       age > s.cfg.FetchWarnAfter(),
	}
}

// remoteRefs lists the remote-tracking refs, which is what freshness reads.
func (s *session) remoteRefs(ctx context.Context) ([]gitx.Ref, error) {
	return s.git.ForEachRef(ctx, remoteRefPattern)
}

// remoteRefPattern is where a fetch writes what it found.
const remoteRefPattern = "refs/remotes/"

// issueDir is where an issue's folder sits, relative to the repository root.
func issueDir(id string) string { return repo.IssuesDir + "/" + id }

// readmePath is an issue's own file, relative to the repository root.
func readmePath(id string) string { return issueDir(id) + "/" + issue.ReadmeName }
