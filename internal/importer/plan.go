package importer

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dgorshkov/isu/internal/issue"
)

// Options is what the caller decides about one import.
type Options struct {
	// Prefix is what ids are formed under. It is .isu.yml's prefix unless
	// --id-prefix said otherwise, for a team importing into a tracker whose
	// own prefix means something else.
	Prefix string
	// Owner is who owns an issue the source left unassigned. With neither, the
	// import refuses — and the dry run says how many issues have neither,
	// which is the number somebody has to act on.
	Owner string
	// Samples is how many issues the dry run shows in full.
	Samples int
	// Evidence is what history and the source say about which commit resolved
	// each issue. It may be nil: an import with nothing to scan writes
	// resolution dates and no links.
	Evidence *Evidence
}

// DefaultSamples is how many issues a dry run shows in full when nothing said.
const DefaultSamples = 2

// Folder is one issue folder an import would write.
type Folder struct {
	// ID is the folder name.
	ID string
	// Key is the source key it was formed from.
	Key string
	// Ref is that key with the repository in front of it, which is what tells
	// two trackers' `#1` apart when both are imported into one repository.
	Ref string
	// Issue is the decoded README, which Validate has already accepted.
	Issue *issue.Issue
	// Files are everything in the folder, README.md first, each name relative
	// to the folder and slash-separated.
	Files []File
}

// File is one file inside an issue folder.
type File struct {
	// Name is relative to the issue folder — `README.md`, `source.yml`,
	// `comments/2026-08-24-alice-01.md`.
	Name string
	// Body is what goes in it.
	Body []byte
}

// Sample is one issue a dry run shows in full, so that "5,000 issues" is not
// the only thing anybody sees before agreeing to write them.
type Sample struct {
	ID     string
	Key    string
	Files  []string
	README string
}

// Report is what a dry run says, and what --json prints.
type Report struct {
	Source     string
	Repository string
	// Items is how many issues the source handed over.
	Items int
	// Folders is how many would be written. It is smaller than Items whenever
	// something was refused.
	Folders int
	// Epics is how many of those are containers rather than tickets.
	Epics int
	// Comments, Attachments and Extra are the three things that would
	// otherwise be lost: comment files, recorded attachment links, and keys in
	// source.yml that isu's schema has no place for.
	Comments    int
	Attachments int
	Extra       int
	// Provenance is how many required fields were written as a provenance line
	// because the source supplied none. Counted so that nobody mistakes
	// archaeology for content.
	Provenance int
	// Unowned is how many issues have neither an assignee nor --owner. It is
	// the number that makes --write refuse.
	Unowned int
	// Dangling is how many parent or blocked_by references pointed outside the
	// import and were recorded in source.yml instead of written as ids that
	// resolve to nothing.
	Dangling int
	// Skipped is what the source declined, and what this mapping declined.
	Skipped []Skip
	// Unplaced are type names and labels no map placed.
	Unplaced []string
	// Found names the source features this repository actually uses.
	Found []string
	// Notes are the sentences a specific source has to say for itself.
	Notes []string
	// Requests is what reading the source cost its rate-limit budget.
	Requests int
	// Types and States are the shape of what is arriving, counted.
	Types  map[string]int
	States map[string]int
	// Evidence is how many issues were linked to a resolving commit, by tier.
	Evidence map[string]int
	// Samples are the issues shown in full.
	Samples []Sample
	// Wrote is the paths written, relative to the repository root. It is empty
	// on a dry run, which is what makes a dry run one.
	Wrote []string
}

// Plan is a whole import, mapped and not yet written.
type Plan struct {
	Mapping *Mapping
	Folders []Folder
	Report  Report
}

// Keys forms the id for every issue in a batch.
//
// It is a separate step from Map because the evidence scan needs the set of
// keys actually being imported before it can read a commit message: GitHub
// numbers issues and pull requests from one sequence, so `(#456)` in a squash
// subject is only a link when 456 is one of these.
func Keys(b *Batch, prefix string) (*Mapping, error) {
	if !issue.ValidID(prefix) {
		return nil, fmt.Errorf(
			"%q cannot be an id prefix: it becomes part of every issue's folder name, "+
				"so it is letters, digits and the three marks - _ and a full stop",
			prefix)
	}

	m := NewMapping(prefix)

	for _, item := range b.Items {
		if _, err := m.Add(item.Key); err != nil {
			return nil, err
		}
	}

	return m, nil
}

// pending is one issue that will be written, before its files are rendered.
//
// The rendering waits because a link is only good once the whole import is
// known: an issue whose epic was refused for want of an owner must not be
// written with a `parent` that resolves to nothing, which is exactly what
// M5-S2 fails a repository for.
type pending struct {
	item  Item
	issue *issue.Issue
	extra map[string]any
}

// Map renders a batch into the folders an import would write.
//
// It writes nothing. Every refusal is counted rather than fatal, because the
// point of a dry run is to say what the whole import would do rather than to
// stop at the first issue somebody forgot to assign.
func Map(b *Batch, m *Mapping, opts Options) (*Plan, error) {
	if opts.Samples == 0 {
		opts.Samples = DefaultSamples
	}

	p := &Plan{Mapping: m, Report: Report{
		Source:     b.Source,
		Repository: b.Repository,
		Items:      len(b.Items),
		Skipped:    append([]Skip{}, b.Skipped...),
		Unplaced:   b.Unplaced,
		Found:      b.Found,
		Notes:      b.Notes,
		Requests:   b.Requests,
		Types:      map[string]int{},
		States:     map[string]int{},
		Evidence:   map[string]int{},
	}}

	var queue []pending

	for _, item := range b.Items {
		one, err := p.build(b, m, item, opts)
		if err != nil {
			return nil, err
		}

		if one != nil {
			queue = append(queue, *one)
		}
	}

	p.prune(m, queue)

	for _, one := range queue {
		rendered, err := files(one.issue, one.extra, one.item)
		if err != nil {
			return nil, err
		}

		p.Folders = append(p.Folders, Folder{
			ID: one.issue.ID, Key: one.item.Key, Ref: one.item.Ref,
			Issue: one.issue, Files: rendered,
		})
	}

	p.summarise(opts)

	return p, nil
}

// build maps one item, or nothing when the item cannot be written.
func (p *Plan) build(b *Batch, m *Mapping, item Item, opts Options) (*pending, error) {
	id, _ := m.ID(item.Key)

	owner := item.Owner
	if owner == "" {
		owner = opts.Owner
	}

	if owner == "" {
		p.Report.Unowned++
		p.Report.Skipped = append(p.Report.Skipped, Skip{
			Ref: item.Ref,
			Why: "nobody is assigned and no --owner was given",
		})

		return nil, nil
	}

	extra := p.extra(b, m, item, opts)

	i := &issue.Issue{
		Schema:     issue.CurrentSchema,
		ID:         id,
		Title:      title(item),
		Type:       item.Type,
		Owner:      owner,
		Created:    item.Created,
		Priority:   item.Priority,
		Reason:     item.Reason,
		Resolution: item.Resolution,
		Body:       body(item),
		Folder:     id,
	}

	if !item.Epic {
		i.State = item.State
	}

	i.Parent, i.BlockedBy = p.links(m, item, extra)
	p.require(i, item)

	if err := i.Validate(); err != nil {
		return nil, fmt.Errorf("%s would not be a valid issue: %w", item.Ref, err)
	}

	return &pending{item: item, issue: i, extra: extra}, nil
}

// prune drops the links that point at an issue this import is not writing.
//
// The keys that were never in the import at all are already gone — links took
// those out — and these are the other half: an issue that was in the import and
// was then refused, most often for want of an owner. Both end up in source.yml
// rather than in frontmatter, because a `parent` naming a folder that does not
// exist is a check failure and a lost fact is worse than either.
func (p *Plan) prune(m *Mapping, queue []pending) {
	written := make(map[string]bool, len(queue))
	for _, one := range queue {
		written[one.issue.ID] = true
	}

	for _, one := range queue {
		var outside []string

		if parent := one.issue.Parent; parent != "" && !written[parent] {
			outside = append(outside, sourceKey(m, parent))
			one.issue.Parent = ""
		}

		keep := make([]string, 0, len(one.issue.BlockedBy))

		for _, id := range one.issue.BlockedBy {
			if written[id] {
				keep = append(keep, id)
				continue
			}

			outside = append(outside, sourceKey(m, id))
		}

		one.issue.BlockedBy = keep

		if len(outside) > 0 {
			p.Report.Dangling += len(outside)
			addOutside(one.extra, outside)
		}
	}
}

// sourceKey names an id the way the source did, so that source.yml records the
// link somebody can actually follow. Every id here was formed by this mapping,
// so the lookup is the whole of it.
func sourceKey(m *Mapping, id string) string {
	key, _ := m.Key(id)

	return key
}

// addOutside records the links that were not written, merging with whatever
// links already recorded.
func addOutside(extra map[string]any, keys []string) {
	was, _ := extra[outsideKey].([]string)
	extra[outsideKey] = append(was, keys...)
}

// outsideKey is where source.yml records a link this import did not write.
const outsideKey = "outside_the_import"

// title is the one-line title, and the reason it is a function is that a source
// is not obliged to hand over a title that is one line or a title at all.
func title(item Item) string {
	line := strings.TrimSpace(strings.ReplaceAll(
		strings.ReplaceAll(item.Title, "\r", " "), "\n", " "))
	if line == "" {
		return item.Ref
	}

	return line
}

// body is the markdown body, ending in exactly one newline.
//
// Nothing in it is rewritten. An attachment link that still resolves on
// github.com is worth more than one this tool rewrote and cannot fetch, and a
// body that survives byte for byte is one an idempotency test can assert.
func body(item Item) string {
	text := strings.TrimRight(item.Body, "\n")
	if text == "" {
		return ""
	}

	return text + "\n"
}

// links maps parent and blocked_by through the import, recording what pointed
// outside it rather than writing an id that resolves to nothing.
func (p *Plan) links(m *Mapping, item Item, extra map[string]any) (string, []string) {
	var (
		parent  string
		blocked []string
		outside []string
	)

	if item.Parent != "" {
		if id, ok := m.ID(item.Parent); ok {
			parent = id
		} else {
			outside = append(outside, item.Parent)
		}
	}

	for _, key := range item.BlockedBy {
		if id, ok := m.ID(key); ok {
			blocked = append(blocked, id)
			continue
		}

		outside = append(outside, key)
	}

	if len(outside) > 0 {
		p.Report.Dangling += len(outside)
		addOutside(extra, outside)
	}

	return parent, blocked
}

// require fills in whichever field the type requires when the source supplied
// none, as a provenance line rather than as invented content.
//
// The rejected alternative was to call everything a chore, which validates
// trivially and throws away the bug/story distinction across the whole history
// in one move.
func (p *Plan) require(i *issue.Issue, item Item) {
	line := "imported from " + item.Ref + "; see the body"

	switch i.Type {
	case issue.TypeBug:
		i.Repro = line
	case issue.TypeStory:
		i.Acceptance = line
	case issue.TypeSpike:
		i.Question = line
	default:
		// A chore and an epic require nothing, and a type this build does not
		// know is Validate's to refuse rather than this function's to invent a
		// field for.
		return
	}

	p.Report.Provenance++
}

// extra is everything about this item that isu's schema has no place for.
func (p *Plan) extra(b *Batch, m *Mapping, item Item, opts Options) map[string]any {
	out := map[string]any{
		"source":     b.Source,
		"repository": b.Repository,
		"key":        item.Key,
		"ref":        item.Ref,
	}

	if item.URL != "" {
		out["url"] = item.URL
	}

	if len(item.Attachments) > 0 {
		out["attachments"] = item.Attachments
		p.Report.Attachments += len(item.Attachments)
	}

	if item.Closing != "" {
		out["closed_by"] = item.Closing
	}

	p.Report.Extra += len(item.Extra)

	for key, value := range item.Extra {
		out[key] = value
	}

	p.evidence(m, item, out, opts)

	return out
}

// evidence records which commit resolved this issue, and which tier said so.
//
// An unlinked issue keeps its resolution date and nothing else, which is the
// honest answer: a link nobody can check is worse than no link.
func (p *Plan) evidence(m *Mapping, item Item, out map[string]any, opts Options) {
	if opts.Evidence == nil {
		return
	}

	id, _ := m.ID(item.Key)

	link, ok := opts.Evidence.Link(id)
	if !ok {
		return
	}

	out["resolved_by"] = link.Commit
	out["resolved_by_evidence"] = link.Tier.String()
}

// summarise counts what the mapping produced.
func (p *Plan) summarise(opts Options) {
	p.Report.Folders = len(p.Folders)

	for _, f := range p.Folders {
		p.Report.Types[string(f.Issue.Type)]++

		if f.Issue.Type == issue.TypeEpic {
			p.Report.Epics++
			p.Report.States["(fold over its children)"]++
		} else {
			p.Report.States[string(f.Issue.State)]++
		}

		for _, file := range f.Files {
			if strings.HasPrefix(file.Name, issue.CommentsDir+"/") {
				p.Report.Comments++
			}
		}

		if len(p.Report.Samples) < opts.Samples {
			p.Report.Samples = append(p.Report.Samples, sample(f))
		}
	}

	if opts.Evidence != nil {
		for tier, n := range opts.Evidence.Counts() {
			p.Report.Evidence[tier.String()] = n
		}
	}

	sort.Strings(p.Report.Unplaced)
}

func sample(f Folder) Sample {
	s := Sample{ID: f.ID, Key: f.Key}

	for _, file := range f.Files {
		s.Files = append(s.Files, file.Name)

		if file.Name == issue.ReadmeName {
			s.README = string(file.Body)
		}
	}

	return s
}

// files renders the whole folder: the issue, everything the schema had no place
// for, and one file per comment.
func files(i *issue.Issue, extra map[string]any, item Item) ([]File, error) {
	out := []File{{Name: issue.ReadmeName, Body: i.Encode()}}

	rendered, err := renderSource(extra)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", item.Ref, err)
	}

	out = append(out, File{Name: SourceFileName, Body: rendered})

	seq := map[string]int{}

	for _, c := range item.Comments {
		day := c.When.UTC().Format(time.DateOnly)
		stem := day + "-" + slug(c.Author)
		seq[stem]++

		out = append(out, File{
			Name: issue.CommentsDir + "/" + fmt.Sprintf("%s-%02d%s", stem, seq[stem], issue.CommentExt),
			Body: []byte(comment(c)),
		})
	}

	return out, nil
}

// comment is one comment file: who wrote it and when, then what they wrote.
//
// The body is fenced off from the header by nothing but a blank line, and that
// is deliberate — a comment containing `---` at the top of the file would open
// frontmatter, and a comment file has none to open. Nothing parses these; they
// are read.
func comment(c Comment) string {
	return fmt.Sprintf("**%s** on %s\n\n%s\n",
		c.Author, c.When.UTC().Format(time.DateOnly), strings.TrimRight(c.Body, "\n"))
}

// slug turns a display name into something a file may be named for.
//
// A comment file is named for its author, and an author's display name is
// whatever they typed into their profile — which is why PLAN.md M7-S2 calls
// this the live attack and not a hypothetical one. Everything outside a narrow
// alphabet becomes a hyphen, so a name that is a path becomes a name that is
// not one, and SafeName still gets the last word.
func slug(name string) string {
	var b strings.Builder

	dash := false

	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)

			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')

			dash = true
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "anon"
	}

	return out
}
