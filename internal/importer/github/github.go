// Package github reads issues out of GitHub Issues.
//
// It is v1.0.0's one importer, and PLAN.md M7 says why it is this one: whoever
// is adopting isu is already in a git repository, that repository is
// overwhelmingly on GitHub, and the issues they want out are therefore sitting
// beside the code they are migrating — no export request, no admin, no licence.
// GitHub also answers for free the two questions a Jira integration had to be
// installed to answer: which pull request closed this issue, and what is this
// issue blocked by.
//
// # The dump is the format
//
// Everything here reads one shape: the REST list as GitHub returns it.
// `--fetch-only` writes that shape verbatim, the tests read one, and an import
// is therefore reproducible without a network and reviewable as a diff. The
// four keys `gh issue list --json` spells differently are accepted as aliases,
// so a dump somebody already has is readable too.
//
// # Pull requests are not issues
//
// The REST list returns both, and they are told apart by the `pull_request`
// key. Dropping them is the first thing this importer does, and skipping that
// is the classic bug in every importer that skips it.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/issue"
)

// Name is what `isu import github` selects.
const Name = "github"

// MilestoneMarker is what separates a milestone's number from an issue's.
//
// Milestones are numbered from their own sequence, so `#3` and milestone 3 are
// different things and must not collide: the epic an import writes for
// milestone 3 is `<PREFIX>-M3` and the issue numbered 3 is `<PREFIX>-3`.
const MilestoneMarker = "M"

// Options is one import.
type Options struct {
	// Repository is owner/repo. It is required when reading from the API and
	// is taken from the dump otherwise.
	Repository string
	// Dump is a saved REST list, read instead of the API.
	Dump []byte
	// State filters what is imported: open, closed or all.
	State string
	// TypeMap places a label or an issue type name onto one of isu's types.
	// GitHub's own defaults are applied first and need no entry.
	TypeMap map[string]issue.Type
	// Fields says whether to read each issue's field values, which is one
	// request per issue and is the only per-issue request this importer makes.
	Fields bool
	// Client is how the API is reached. It is nil when Dump is set.
	Client *Client
}

// The states an import may ask for.
const (
	StateOpen   = "open"
	StateClosed = "closed"
	StateAll    = "all"
)

// States lists them, which is what --state accepts.
var States = []string{StateOpen, StateClosed, StateAll}

// Source is the importer.Source this package provides.
type Source struct {
	opts Options
}

// New returns a source, refusing an option that cannot work.
func New(opts Options) (*Source, error) {
	if opts.State == "" {
		opts.State = StateAll
	}

	if !known(opts.State, States) {
		return nil, fmt.Errorf("--state is one of %s, and not %q",
			strings.Join(States, ", "), opts.State)
	}

	if opts.Dump == nil && opts.Client == nil {
		return nil, fmt.Errorf(
			"reading %s needs either a saved dump or somewhere to fetch from",
			Name)
	}

	if opts.Dump == nil && opts.Repository == "" {
		return nil, fmt.Errorf("%s needs a repository, as owner/repo", Name)
	}

	return &Source{opts: opts}, nil
}

func known(value string, of []string) bool {
	for _, v := range of {
		if v == value {
			return true
		}
	}

	return false
}

// Name is the source's name.
func (s *Source) Name() string { return Name }

// Describe is the line the help prints.
func (s *Source) Describe() string {
	return "GitHub Issues, from the API or from a saved dump"
}

// Keys finds GitHub's own issue keys in prose.
//
// It is deliberately generous — `#1234` and `owner/repo#1234` both count, and
// so does a bare number in parentheses at the end of a squash subject — because
// every match is checked against the set of issues actually being imported and
// discarded when it is not one of them. That check is the safety, and this is
// only the net.
func (s *Source) Keys(text string) []string {
	var out []string

	seen := map[string]bool{}

	for at := strings.IndexByte(text, '#'); at >= 0; at = strings.IndexByte(text, '#') {
		text = text[at+1:]

		digits := 0
		for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
			digits++
		}

		if digits == 0 {
			continue
		}

		key := "#" + text[:digits]
		if !seen[key] {
			seen[key] = true

			out = append(out, key)
		}
	}

	return out
}

// Load reads everything the repository has.
func (s *Source) Load(ctx context.Context) (*importer.Batch, error) {
	raw, repository, requests, err := s.read(ctx)
	if err != nil {
		return nil, err
	}

	b := &importer.Batch{
		Source:     Name,
		Repository: repository,
		Requests:   requests,
	}

	issues := s.keep(b, raw)

	ix := newIndex(issues)

	for _, in := range issues {
		b.Items = append(b.Items, s.item(b, repository, in, ix))
	}

	b.Items = append(b.Items, ix.epics(repository)...)

	s.found(b, issues, ix)
	b.Notes = append(b.Notes,
		"attachment links are recorded, not fetched: resolving one still needs "+
			"github.com, because on a private repository the asset wants a browser session")

	sort.Strings(b.Unplaced)

	return b, nil
}

// read gets the issue list, from the dump or from the API.
func (s *Source) read(ctx context.Context) ([]Issue, string, int, error) {
	if s.opts.Dump != nil {
		issues, repository, err := parseDump(s.opts.Dump)
		if err != nil {
			return nil, "", 0, err
		}

		if s.opts.Repository != "" {
			repository = s.opts.Repository
		}

		return issues, repository, 0, nil
	}

	issues, err := s.opts.Client.Issues(ctx, s.opts.Repository, s.opts.State, s.opts.Fields)
	if err != nil {
		return nil, "", 0, err
	}

	return issues, s.opts.Repository, s.opts.Client.Requests(), nil
}

// keep drops the pull requests and whatever --state excluded.
func (s *Source) keep(b *importer.Batch, raw []Issue) []Issue {
	out := make([]Issue, 0, len(raw))

	for _, in := range raw {
		in.normalise()

		switch {
		case in.isPullRequest():
			b.Skipped = append(b.Skipped, importer.Skip{
				Ref: "#" + strconv.Itoa(in.Number),
				Why: "a pull request, which the REST list returns beside the issues",
			})
		case s.opts.State != StateAll && in.state() != s.opts.State:
			b.Skipped = append(b.Skipped, importer.Skip{
				Ref: "#" + strconv.Itoa(in.Number),
				Why: "--state " + s.opts.State + " does not ask for it",
			})
		default:
			out = append(out, in)
		}
	}

	return out
}

// found names the features this repository actually uses. A repository with
// none of them still imports, and the dry run says which it found.
func (s *Source) found(b *importer.Batch, issues []Issue, ix *index) {
	features := map[string]bool{}

	for _, in := range issues {
		features["issue types"] = features["issue types"] || in.Type != nil
		features["milestones"] = features["milestones"] || in.Milestone != nil
		features["sub-issues"] = features["sub-issues"] || len(in.SubIssues) > 0
		features["dependencies"] = features["dependencies"] || len(in.BlockedBy) > 0
		features["issue fields"] = features["issue fields"] || len(in.Fields) > 0
		features["comments"] = features["comments"] || len(in.comments) > 0
	}

	for name, used := range features {
		if used {
			b.Found = append(b.Found, name)
		}
	}

	sort.Strings(b.Found)

	if ix.sized > 0 {
		b.Notes = append(b.Notes, fmt.Sprintf(
			"the sub-issue hierarchy is %d levels deep at most and %d issues across; "+
				"isu has one parent and the milestone is spending it, so the tree is "+
				"recorded whole in %s and modelled not at all",
			ix.deepest, ix.sized, importer.SourceFileName))
	}
}

// item maps one GitHub issue onto isu's schema.
func (s *Source) item(
	b *importer.Batch, repository string, in Issue, ix *index,
) importer.Item {
	key := "#" + strconv.Itoa(in.Number)

	item := importer.Item{
		Key:         key,
		Ref:         repository + key,
		URL:         in.HTMLURL,
		Title:       in.Title,
		Type:        s.typeOf(b, in),
		Owner:       assignee(in),
		Created:     in.CreatedAt,
		Body:        in.Body,
		Extra:       map[string]any{},
		Comments:    comments(in),
		Attachments: attachments(in.Body),
		Closing:     closing(in),
	}

	item.State, item.Reason, item.Resolution = state(in)

	if in.Milestone != nil && ix.wanted(in.Milestone.Number) {
		item.Parent = ix.key(in.Milestone.Number)
	}

	for _, blocker := range in.BlockedBy {
		item.BlockedBy = append(item.BlockedBy, dependencyKey(repository, blocker))
	}

	s.extra(&item, in, ix)

	return item
}

// extra is everything GitHub said that isu's schema has no place for.
func (s *Source) extra(item *importer.Item, in Issue, ix *index) {
	if in.User != nil && in.User.Login != "" {
		// The author is deliberately not the owner: the person who filed a bug
		// is usually not the person answerable for it, and M5-S4 makes owner
		// expensive to correct afterwards.
		item.Extra["author"] = in.User.Login
	}

	if len(in.Assignees) > 1 {
		item.Extra["assignees"] = logins(in.Assignees)
	}

	if names := labelNames(in.Labels); len(names) > 0 {
		item.Extra["labels"] = names
	}

	if in.Type != nil {
		item.Extra["issue_type"] = in.Type.Name
	}

	if in.ClosedAt != nil {
		item.Extra["closed_at"] = in.ClosedAt.UTC().Format(time.RFC3339)
	}

	if in.StateReason != "" {
		item.Extra["state_reason"] = in.StateReason
	}

	if in.Milestone != nil {
		item.Extra["milestone"] = in.Milestone.Title
	}

	if fields := fieldValues(in.Fields); len(fields) > 0 {
		item.Extra["issue_fields"] = fields
	}

	if tree := ix.hierarchy(in.Number); tree != nil {
		// The largest thing v1.0.0 knowingly declines to model. isu has one
		// `parent`, it must name an epic, and the milestone is spending it — so
		// a tree eight levels deep is recorded whole rather than flattened
		// into a shape that would misrepresent it.
		item.Extra["sub_issues"] = tree
	}

	if parent, ok := ix.parentOf(in.Number); ok {
		item.Extra["sub_issue_of"] = "#" + strconv.Itoa(parent)
	}
}

// typeOf places an issue onto one of isu's types.
//
// The organisation's issue type where the repository has one, a configured
// label map where it does not, `chore` where neither answers. GitHub's own
// defaults map Bug to bug, Feature to story and Task to chore; every other type
// and every label is the map's business, and the dry run lists what it could
// not place.
func (s *Source) typeOf(b *importer.Batch, in Issue) issue.Type {
	if in.Type != nil {
		if t, ok := s.place(in.Type.Name); ok {
			return t
		}

		b.Unplaced = appendOnce(b.Unplaced, in.Type.Name)

		return issue.TypeChore
	}

	for _, name := range labelNames(in.Labels) {
		if t, ok := s.place(name); ok {
			return t
		}
	}

	b.Unplaced = appendOnce(b.Unplaced, labelNames(in.Labels)...)

	return issue.TypeChore
}

// defaultTypes are GitHub's own issue types, which need no map entry.
var defaultTypes = map[string]issue.Type{
	"bug":     issue.TypeBug,
	"feature": issue.TypeStory,
	"task":    issue.TypeChore,
}

func (s *Source) place(name string) (issue.Type, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))

	if t, ok := s.opts.TypeMap[lower]; ok {
		return t, true
	}

	t, ok := defaultTypes[lower]

	return t, ok
}

func appendOnce(into []string, names ...string) []string {
	for _, name := range names {
		if !known(name, into) {
			into = append(into, name)
		}
	}

	return into
}

// state maps GitHub's open/closed and its state_reason onto isu's three states.
//
// `closed` with no state_reason at all is `resolved` rather than unknown: the
// field only exists since 2022, and everything older is an ordinary closed
// issue. `reopened` becomes `open`, because isu derives reopening from trunk
// history and will not read it from a field it would then have to keep in sync.
func state(in Issue) (issue.State, string, issue.Resolution) {
	if in.state() != StateClosed {
		return issue.StateOpen, "", ""
	}

	switch in.StateReason {
	case "not_planned":
		return issue.StateDropped, "closed on GitHub as not planned", issue.ResolutionWontfix
	case "duplicate":
		return issue.StateDropped, "closed on GitHub as a duplicate", issue.ResolutionDuplicate
	default:
		return issue.StateResolved, "", ""
	}
}

// assignee is the first assignee's login, and the empty string when nobody is
// assigned — which is what makes --owner the next answer and a refusal the one
// after that.
func assignee(in Issue) string {
	for _, a := range in.Assignees {
		if a.Login != "" {
			return a.Login
		}
	}

	return ""
}

func logins(actors []Actor) []any {
	out := make([]any, 0, len(actors))
	for _, a := range actors {
		out = append(out, a.Login)
	}

	return out
}

func labelNames(labels []Label) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Name != "" {
			out = append(out, l.Name)
		}
	}

	return out
}

// dependencyKey names a blocker the way the mapping will look it up.
//
// A blocker in another repository is named for it, so that `other/repo#5` and
// this repository's own `#5` are never the same key — and so that the blocker
// outside the import stays outside it rather than resolving to the wrong issue.
func dependencyKey(repository string, d Dependency) string {
	key := "#" + strconv.Itoa(d.Number)
	if d.Repository != "" && d.Repository != repository {
		return d.Repository + key
	}

	return key
}

// parseDump reads a saved REST list.
//
// Two shapes are accepted: the array the API returns, which is what
// `--fetch-only` writes and what `gh api repos/{owner}/{repo}/issues` prints,
// and an object with the array under `issues`, which is what a dump carrying
// the repository's own name looks like.
func parseDump(data []byte) ([]Issue, string, error) {
	trimmed := strings.TrimLeft(string(data), " \t\r\n")

	if strings.HasPrefix(trimmed, "[") {
		var issues []Issue
		if err := json.Unmarshal(data, &issues); err != nil {
			return nil, "", fmt.Errorf("reading the dump: %w", err)
		}

		return issues, repositoryOf(issues), nil
	}

	var wrapped struct {
		Repository string  `json:"repository"`
		Issues     []Issue `json:"issues"`
	}

	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, "", fmt.Errorf("reading the dump: %w", err)
	}

	repository := wrapped.Repository
	if repository == "" {
		repository = repositoryOf(wrapped.Issues)
	}

	return wrapped.Issues, repository, nil
}

// repositoryOf recovers owner/repo from an issue's own URL, so that a dump
// carrying nothing but the array still knows what it is a dump of.
func repositoryOf(issues []Issue) string {
	for _, in := range issues {
		// Before normalise has run, so both spellings are read here: this is
		// what tells a dump written by `gh` what repository it is a dump of.
		at := in.HTMLURL
		if at == "" {
			at = in.URLAlias
		}

		if _, rest, ok := strings.Cut(at, "github.com/"); ok {
			parts := strings.Split(rest, "/")
			if len(parts) >= 2 {
				return parts[0] + "/" + parts[1]
			}
		}
	}

	return ""
}
