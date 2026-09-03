// Package importer turns another tracker's issues into isu's.
//
// # The seam
//
// A Source hands over a Batch of Items and knows nothing about folders, ids or
// git. Everything after that — forming ids, mapping fields, deciding what a
// dry run says, writing anything to disk — happens here, once, for every
// source there will ever be. v1.0.0 ships one importer (GitHub Issues), and
// this interface is the seam PLAN.md M7 says Jira and Linear come back
// through: a second source is a Load method, not a second importer.
//
// # The mapping is total and reversible
//
// Every field a source hands over lands somewhere. The ones isu's schema has a
// place for become frontmatter; **everything else becomes source.yml in the
// issue's own folder and never touches frontmatter**, because a mature tracker
// has two hundred custom fields and a schema that absorbs them is not a schema.
// The id is formed from the source's own key and the Mapping reads it back, so
// the thing a team has been writing in commit messages for years is still
// legible in the id and still recoverable from it.
//
// # Nothing here writes a file except Writer
//
// An importer writes attacker-influenced data into somebody's repository —
// ticket titles, comment bodies, author display names — so there is exactly one
// guarded write path (safe.go) and a grep test in the style of M2-S1 that fails
// the build when anything else in this package touches the filesystem.
package importer

import (
	"context"
	"time"

	"github.com/dgorshkov/isu/internal/issue"
)

// Comment is one comment on a source issue.
//
// Author and When are what the comment file is named for, and both are outside
// the importer's control — a display name is whatever somebody typed into
// their profile — so both go through the same sanitiser every other name does.
type Comment struct {
	// Author is the display name or login the source reported.
	Author string
	// When is when it was written, which is the first half of its file name.
	When time.Time
	// Body is the markdown, verbatim.
	Body string
}

// Item is one issue as a source hands it over.
//
// The fields down to Body are the ones isu's schema has a place for. Extra is
// everything else the source said, and it is the reason this type does not
// grow a column every time a tracker invents a feature.
type Item struct {
	// Key is the source's own name for the issue — PROJ-1234, #1234. It is
	// what the id is formed from and what the evidence scan matches against.
	Key string
	// Ref is the key as a human writes it, repository and all —
	// owner/repo#1234 — and is what a provenance line names.
	Ref string
	// URL is where the issue still lives.
	URL string

	Title      string
	Type       issue.Type
	State      issue.State
	Owner      string
	Created    time.Time
	Priority   issue.Priority
	Reason     string
	Resolution issue.Resolution
	// Parent is the source key of the epic this belongs to, and is mapped to
	// an id once the whole import is in hand: an epic outside the import is
	// not written as an id that resolves to nothing.
	Parent string
	// BlockedBy are source keys, mapped the same way.
	BlockedBy []string
	// Body is the markdown body, verbatim. An importer rewrites nothing in it:
	// an attachment link that still resolves on github.com is worth more than
	// one this tool rewrote and cannot fetch.
	Body string

	// Extra is everything isu's schema has no place for. It becomes
	// source.yml.
	Extra map[string]any
	// Comments become files under comments/.
	Comments []Comment
	// Attachments are the links found in the body. They are recorded, never
	// fetched — see PLAN.md M7-S5.
	Attachments []string
	// Closing is the pull request or commit the source itself says closed this
	// issue. It is the strongest evidence tier there is, and it arrives with
	// the issue rather than through an integration somebody installed.
	Closing string
	// Epic says this item is a container rather than a ticket: it is written
	// with no state, and its status is the fold over its children.
	Epic bool
}

// Skip is one thing a source declined to import, and why. The dry run lists
// them: an importer that silently drops rows is one nobody can audit.
type Skip struct {
	// Ref names what was skipped, as a human writes it.
	Ref string
	// Why is the reason, in one clause.
	Why string
}

// Batch is everything one source loaded.
type Batch struct {
	// Source is the source's name, which is also what `isu import <name>`
	// selects.
	Source string
	// Repository is what the source calls the project — owner/repo.
	Repository string
	// Items are the issues, in the order the source read them, which is the
	// order ids are formed in.
	Items []Item
	// Skipped is what the source declined and why.
	Skipped []Skip
	// Unplaced are the source's own type names and labels that no map placed,
	// so that the dry run can say what fell through to the default.
	Unplaced []string
	// Found names the source features this repository actually uses — issue
	// types, sub-issues, dependencies, issue fields. A repository with none of
	// them still imports, and the dry run says which it found.
	Found []string
	// Requests is what reading this cost the source's rate-limit budget.
	Requests int
	// Notes are sentences the dry run prints verbatim, for the things a
	// specific source has to say for itself.
	Notes []string
}

// Source is where issues come from.
//
// Four methods, and three of them are description: the interface is small
// because everything that is the same for every tracker — ids, folders, the
// dry run, the guarded write — is on this side of it.
type Source interface {
	// Name is what `isu import <name>` selects.
	Name() string
	// Describe is the one line the help prints.
	Describe() string
	// Load reads everything the source has.
	Load(ctx context.Context) (*Batch, error)
	// Keys finds this source's own issue keys in a line of prose, which is what
	// the evidence scan matches commit messages with. A key that is not in the
	// import is discarded by the scan, so this may be generous.
	Keys(text string) []string
}
