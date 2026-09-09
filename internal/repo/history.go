package repo

import (
	"context"
	"errors"
	"time"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
)

// StateAt is one trunk commit at which an issue's state changed.
type StateAt struct {
	// State is what the file said after this commit. It is empty on an epic,
	// which declares none, and empty when Removed — which is why Removed is a
	// field rather than something to infer from an empty state.
	State issue.State
	// Removed says the issue's file was deleted at this commit.
	Removed bool
	// Commit is the trunk commit the change landed in.
	Commit string
	// When is that commit's committer date, which is when trunk changed. The
	// author date is when the work was written, and a squash merge rewrites it
	// to something else again; neither answers "when did this land".
	When time.Time
}

// Terminal reports whether the issue was in a state it does not come back from
// on its own.
func (s StateAt) Terminal() bool { return !s.Removed && s.State.Terminal() }

// History is, for each issue, the ordered sequence of states its file has held
// at trunk. Oldest first.
type History map[string][]StateAt

// LoadHistory reads the states every issue's file has held at trunk.
//
// `reopened` is one of the six statuses in the plan, and the only one that
// cannot be answered from the current content of any ref. Walking history per
// issue inside the derivation package would be a git process per issue and
// would make M3-S1's "no git calls inside" rule a lie the moment M3-S4 landed.
// So the walk happens here and derivation stays pure over what it is handed.
//
// Two processes, whatever the repository holds: one `git log --raw`, which
// names the blob at every change, and one `cat-file --batch` fed those blobs.
//
// Two things about the walk are load-bearing.
//
// --first-parent is trunk's own timeline. A merge commit shows no diff of its
// own, and under a pathspec git simplifies it away and reports the change at
// the branch commit instead — a commit that was never on trunk, carrying the
// date the work was written rather than the date it landed. With
// --first-parent the merge reports what it brought in, which is what "the
// states the file has held at trunk" means.
//
// The state comes from blob content and never from commit metadata. Squash
// collapses authorship; it does not touch the file. A squash-only history and
// a merge-commit history of the same logical changes therefore read the same,
// which is the property M3-S5 depends on.
//
// Renames are not followed. A moved issue folder is one issue ending and
// another beginning, because following it means asking git to guess which of
// two issues a file became, and a wrong guess silently rewrites somebody's
// history. This is a documented limitation, and the tests assert it rather
// than working around it.
func (r *Repo) LoadHistory(ctx context.Context, trunk string) (History, error) {
	commits, err := r.git.Log(ctx, gitx.LogSpec{
		Rev:         trunk,
		Paths:       []string{IssuesDir},
		FirstParent: true,
		Raw:         true,
		Reverse:     true,
	})
	if err != nil {
		if trunk == unbornHEAD && errors.Is(err, gitx.ErrUnknownRevision) {
			return History{}, nil
		}

		return nil, err
	}

	changes, oids := issueChanges(commits)

	states, err := r.statesOf(ctx, oids)
	if err != nil {
		return nil, err
	}

	return assemble(changes, states), nil
}

// change is one commit's effect on one issue, before the blob has been read.
type change struct {
	id      string
	oid     string
	removed bool
	commit  string
	when    time.Time
}

// issueChanges keeps the changes that are issue READMEs and collects the blobs
// they name, deduplicated: an issue reverted to a state it held before is the
// same blob again, and so are two issues whose files happen to be identical.
func issueChanges(commits []gitx.Commit) ([]change, []string) {
	var (
		changes []change
		oids    []string
	)

	seen := map[string]bool{}

	for _, commit := range commits {
		for _, c := range commit.Changes {
			id, ok := issueID(c.Path)
			if !ok || !issue.ValidID(id) {
				continue
			}

			changes = append(changes, change{
				id:      id,
				oid:     c.NewOID,
				removed: c.Deleted(),
				commit:  commit.OID,
				when:    commit.Committer.When,
			})

			if !c.Deleted() && !seen[c.NewOID] {
				seen[c.NewOID] = true
				oids = append(oids, c.NewOID)
			}
		}
	}

	return changes, oids
}

// statesOf reads the state field out of every blob in one batch.
//
// A blob that does not parse contributes no state and does not stop the walk.
// Somebody's broken commit in the middle of last year is not a reason for the
// board to refuse to render today, and the states either side of it are still
// the states the file held.
func (r *Repo) statesOf(ctx context.Context, oids []string) (map[string]issue.State, error) {
	states := make(map[string]issue.State, len(oids))

	err := r.git.CatFileBatch(ctx, oids, func(o gitx.Object) error {
		doc, err := issue.Parse(o.Data)
		if err != nil {
			return nil
		}

		state, _ := doc.Get(issue.KeyState)
		states[o.OID] = issue.State(state)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return states, nil
}

// assemble folds the changes into one sequence per issue, recording a commit
// only where the state actually changed.
//
// Collapsing is what keeps the sequence about states rather than about commits.
// An issue whose title was fixed twice has not changed state twice, and a
// `reopened` answered by scanning that noise would be answered by counting
// typos.
func assemble(changes []change, states map[string]issue.State) History {
	history := History{}

	for _, c := range changes {
		// Absent from the map means the blob did not parse, which is not a
		// state. Present and empty means it parsed and declared none, which is
		// what an epic looks like — so the two are told apart by the lookup and
		// not by the value.
		state, parsed := states[c.oid]
		if !c.removed && !parsed {
			continue
		}

		at := StateAt{
			State:   state,
			Removed: c.removed,
			Commit:  c.commit,
			When:    c.when,
		}

		previous := history[c.id]
		if n := len(previous); n > 0 {
			last := previous[n-1]
			if last.State == at.State && last.Removed == at.Removed {
				continue
			}
		}

		history[c.id] = append(previous, at)
	}

	return history
}
