package repo

import (
	"context"
	"sort"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
)

// Edit is what one commit did to one issue's file.
//
// Before and After are the file on either side of that commit, decoded. They
// are the reason this exists rather than a list of paths: the two questions M5
// asks about a commit — did it change `owner:`, did it resolve something — are
// questions about the values in the file, and a path cannot answer either.
type Edit struct {
	// ID is the issue.
	ID string
	// Path is its README, relative to the repository root.
	Path string
	// Added says this commit created the file.
	Added bool
	// Removed says this commit deleted it.
	Removed bool
	// Before is the file as this commit found it, and is nil when the commit
	// added it or when that copy would not decode.
	Before *issue.Issue
	// After is the file as this commit left it, and is nil when the commit
	// removed it or when that copy would not decode.
	After *issue.Issue
}

// Commit is one commit on the branch under review.
//
// The author is here because M5-S4 is about authorship and nothing else: an
// agent may not change an owner, and a human may. It is the author rather than
// the committer, because a rebase rewrites the committer of work somebody else
// wrote.
type Commit struct {
	// OID is the commit.
	OID string
	// Author is who wrote it and when.
	Author gitx.Signature
	// Subject is the first line of its message.
	Subject string
	// Edits are what it did to issue files, in path order. A commit that
	// touched nothing under issues/ has none, and is still here: a branch's
	// authorship is a fact about all of its commits.
	Edits []Edit
}

// IssueChange is what a branch did to one issue overall.
//
// It is folded over the branch's commits rather than diffed at the ends,
// because the two answer different questions and only this one is about the
// branch: what its first commit found there, and what its last one left.
type IssueChange struct {
	// ID is the issue.
	ID string
	// Path is its README, relative to the repository root.
	Path string
	// Added says the branch created the folder — an issue trunk has never seen.
	Added bool
	// Removed says the branch deleted it.
	Removed bool
	// Before is trunk's copy as the branch found it, or nil when the branch
	// created it or that copy would not decode.
	Before *issue.Issue
	// After is what the branch leaves behind, or nil when it removed the file
	// or the result would not decode.
	After *issue.Issue
}

// Branch is what one ref proposes over trunk.
//
// This is the "diff against trunk" PLAN.md M5-S1 hands to every check, and it
// is loaded here for the same reason everything else is: a check that could
// spawn a git process would spawn one per issue the first time somebody was in
// a hurry.
type Branch struct {
	// Trunk is what it was compared against.
	Trunk string
	// Head is the ref under review.
	Head string
	// Base is where the two diverged. Every difference here is measured from
	// it, so a trunk that has moved on since the branch left does not read as
	// the branch reverting work it never touched.
	Base string
	// Commits are the branch's own commits, oldest first.
	Commits []Commit
	// Paths is every path that differs between the base and the head, sorted.
	// It is the whole tree and not only issues/: the evidence check in M5-S3 is
	// precisely a question about whether anything else changed.
	Paths []string
	// Issues is what the branch did to each issue it touched, in id order.
	Issues []IssueChange
}

// Outside lists the changed paths that are not under a directory.
//
// Resolving a bug, a story or a chore requires a change outside issues/. That
// sentence is the whole of M5-S3, and this is it as a question about a diff.
func (b *Branch) Outside(dir string) []string {
	var out []string

	for _, path := range b.Paths {
		if !under(path, dir) {
			out = append(out, path)
		}
	}

	return out
}

// Change is what the branch did to one issue, and whether it touched it at all.
func (b *Branch) Change(id string) (IssueChange, bool) {
	for _, c := range b.Issues {
		if c.ID == id {
			return c, true
		}
	}

	return IssueChange{}, false
}

// LoadBranch reads what a ref proposes over trunk.
//
// Three processes: one merge-base, one diff, and one log — plus a single
// `cat-file --batch` for every issue file the branch's commits touched, on
// either side of each. That is the read path in PLAN.md §0 applied to a branch:
// the log names the blobs, so nothing here walks a tree per lookup.
func (r *Repo) LoadBranch(ctx context.Context, trunk, head string) (*Branch, error) {
	base, err := r.git.MergeBase(ctx, trunk, head)
	if err != nil {
		return nil, err
	}

	paths, err := r.git.DiffNameOnly(ctx, base, head)
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)

	commits, err := r.git.Log(ctx, gitx.LogSpec{
		Rev:     base + ".." + head,
		Raw:     true,
		Reverse: true,
	})
	if err != nil {
		return nil, err
	}

	branch := &Branch{Trunk: trunk, Head: head, Base: base, Paths: paths}

	cache := newBlobCache()

	for _, commit := range commits {
		for _, change := range commit.Changes {
			if _, ok := issueID(change.Path); !ok {
				continue
			}

			if !added(change) {
				cache.want(change.OldOID)
			}
			if !change.Deleted() {
				cache.want(change.NewOID)
			}
		}
	}

	if err := cache.fill(ctx, r.git); err != nil {
		return nil, err
	}

	for _, commit := range commits {
		branch.Commits = append(branch.Commits, Commit{
			OID:     commit.OID,
			Author:  commit.Author,
			Subject: commit.Subject,
			Edits:   cache.edits(commit),
		})
	}

	branch.Issues = fold(branch.Commits)

	return branch, nil
}

// added reports whether a change created the path. The letter is git's, and it
// is read rather than the object id compared against forty zeroes: the width of
// that id depends on the repository's hash algorithm and the letter does not.
func added(c gitx.Change) bool { return c.Status != "" && c.Status[0] == 'A' }

// edits turns one commit's raw changes into the issue files it edited.
func (c *blobCache) edits(commit gitx.Commit) []Edit {
	var edits []Edit

	for _, change := range commit.Changes {
		id, ok := issueID(change.Path)
		if !ok {
			continue
		}

		edit := Edit{
			ID:      id,
			Path:    change.Path,
			Added:   added(change),
			Removed: change.Deleted(),
		}

		if !edit.Added {
			edit.Before = c.issue(id, change.OldOID)
		}
		if !edit.Removed {
			edit.After = c.issue(id, change.NewOID)
		}

		edits = append(edits, edit)
	}

	sort.SliceStable(edits, func(a, b int) bool { return edits[a].Path < edits[b].Path })

	return edits
}

// issue decodes one blob, or returns nil for a file that will not decode.
//
// A branch carrying an unreadable issue file is not a load failure: the schema
// check is what reports it, and it can only report it if the loader hands the
// rest of the branch over.
func (c *blobCache) issue(id, oid string) *issue.Issue {
	key := decodeKey{oid: oid, id: id}

	if known, ok := c.decoded[key]; ok {
		return known
	}
	if _, known := c.broken[key]; known {
		return nil
	}

	decoded, err := decode(id, c.data[oid])
	if err != nil {
		c.broken[key] = err

		return nil
	}

	c.decoded[key] = decoded

	return decoded
}

// fold reduces a branch's commits to one change per issue: the first commit's
// starting point and the last one's result.
func fold(commits []Commit) []IssueChange {
	changes := map[string]*IssueChange{}

	var order []string

	for _, commit := range commits {
		for _, edit := range commit.Edits {
			change, seen := changes[edit.ID]
			if !seen {
				change = &IssueChange{
					ID: edit.ID, Path: edit.Path, Added: edit.Added, Before: edit.Before,
				}
				changes[edit.ID] = change
				order = append(order, edit.ID)
			}

			change.Removed = edit.Removed
			change.After = edit.After
		}
	}

	sort.Strings(order)

	out := make([]IssueChange, 0, len(order))
	for _, id := range order {
		out = append(out, *changes[id])
	}

	return out
}

// under reports whether a path is inside a directory.
func under(path, dir string) bool {
	return len(path) > len(dir)+1 && path[:len(dir)] == dir && path[len(dir)] == '/'
}
