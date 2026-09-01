package check

import (
	"sort"
	"strings"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// Copy is one ref's copy of one issue file.
//
// The structural rules are about files rather than about issues, and the
// difference matters as soon as there is more than one ref: an issue whose file
// is fine at trunk and broken on the branch proposing it is a broken issue, and
// a check that only ever read what derivation chose to render would call it
// fine. So the rules walk every distinct copy.
type Copy struct {
	// Ref is where it was read: the name this run calls trunk, or a branch's
	// full ref name.
	Ref string
	// OnTrunk says it is trunk's copy, which is the one that is true.
	OnTrunk bool
	// ID is the folder name.
	ID string
	// Path is the file, relative to the repository root.
	Path string
	// Issue is the decoded file, and is nil when it would not decode.
	Issue *issue.Issue
	// Err is why it would not.
	Err error
}

// Where puts a message where it happened.
//
// On trunk it says nothing: trunk is where state is true, and a report that
// prefixed every finding with the name of the branch everybody is on would be
// noise around the half of them that matter most.
func (c Copy) Where(message string) string {
	if c.OnTrunk {
		return message
	}

	return "on " + shortRef(c.Ref) + ": " + message
}

// Copies is every distinct copy of every issue the loaded refs hold: trunk's,
// and each branch's own where it differs from trunk's.
//
// A branch that did not touch an issue contributes nothing here, which is what
// keeps this proportional to what the branches actually changed rather than to
// branches times issues. It is the same bargain repo.Board.Changed makes, and
// the reason that field exists.
func (in Input) Copies() []Copy {
	if in.Loaded == nil {
		return nil
	}

	out := trunkCopies(in)

	for _, ref := range in.Loaded.Names() {
		set := in.Loaded.Refs[ref]

		for _, id := range in.Loaded.Changed[ref] {
			if i, ok := set.Get(id); ok {
				out = append(out, Copy{
					Ref: ref, ID: id, Path: readmePath(id), Issue: i,
				})

				continue
			}

			// Not readable at this ref: either the branch deleted the folder,
			// which is not a copy of anything, or the file will not decode,
			// which is the copy that most needs reporting.
			for _, broken := range set.Broken {
				if broken.ID == id {
					out = append(out, Copy{
						Ref: ref, ID: id, Path: broken.Path, Err: broken.Err,
					})
				}
			}
		}
	}

	return out
}

// trunkCopies is trunk's own, in id order, readable and unreadable together.
func trunkCopies(in Input) []Copy {
	set := in.Loaded.Trunk

	out := make([]Copy, 0, set.Len()+len(set.Broken))

	for _, id := range set.IDs() {
		i, _ := set.Get(id)
		out = append(out, Copy{
			Ref: in.Trunk, OnTrunk: true, ID: id, Path: readmePath(id), Issue: i,
		})
	}

	for _, broken := range set.Broken {
		out = append(out, Copy{
			Ref: in.Trunk, OnTrunk: true, ID: broken.ID, Path: broken.Path, Err: broken.Err,
		})
	}

	sort.SliceStable(out, func(a, b int) bool { return out[a].ID < out[b].ID })

	return out
}

// Issues walks the board's issues in id order, which is what the rules about
// one repository rather than about one file are written over.
func (in Input) Issues(fn func(id string, i *issue.Issue)) {
	if in.Board == nil {
		return
	}

	for _, id := range in.Board.IDs() {
		item, _ := in.Board.Get(id)
		if item.Issue == nil {
			// A folder trunk carries whose file nothing can read. The schema
			// rule is what reports that; every rule below it would be asking
			// questions of fields nobody can see.
			continue
		}

		fn(id, item.Issue)
	}
}

// readmePath is an issue's own file, relative to the repository root.
func readmePath(id string) string {
	return repo.IssuesDir + "/" + id + "/" + issue.ReadmeName
}

// shortRef is a ref name as a person writes it.
func shortRef(ref string) string { return strings.TrimPrefix(ref, repo.DefaultRefPattern) }
