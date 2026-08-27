package repo

import (
	"context"

	"github.com/dgorshkov/isu/internal/gitx"
)

// FirstCommit is the first commit on a branch, counting from trunk.
//
// On a claiming branch it is the commit `isu claim` wrote: the one that flipped
// the state to resolved before any work started. Its author is the claimant and
// its author date is the claim time.
type FirstCommit struct {
	// Ref is the branch this was the first commit on.
	Ref string
	// OID is the commit.
	OID string
	// Author is who wrote it and when. The author date is the claim time — the
	// committer date is rewritten by a rebase, and a claim that aged whenever
	// somebody rebased would be a claim nobody could trust.
	Author gitx.Signature
	// Subject is the first line of its message, which `isu claim` writes as
	// `claim <ID>`.
	Subject string
}

// LoadFirstCommits reads the first commit on each of the given refs.
//
// This is the one thing claiming by branch costs that a dedicated claim ref
// would have made free: a claim ref *is* the claim commit, where a branch has
// to be asked which of its commits was. It is one git process per claiming
// branch — linear in refs, which is what the board already costs — and the
// caller passes only the refs that claim something, so the branches doing
// ordinary work cost nothing.
//
// The branch tip is free from for-each-ref and is the wrong answer: it moves
// every time the claimant pushes more work, so a claim would never age and
// stale_days would never fire.
//
// PLAN.md spells this `--reverse --max-count=1`, and that pair returns the tip.
// Git applies the limit during the walk, which starts at the tip, and reverses
// what survived it; one commit reversed is that same commit. So the range is
// walked and the first record taken, which costs the branch's own commits
// rather than a constant — affordable, because a claiming branch is a few
// commits, and correct, which the pair is not at any price. The tests in
// claims_test.go hold this, and PLAN.md is corrected to match.
func (r *Repo) LoadFirstCommits(
	ctx context.Context, trunk string, refs []string,
) (map[string]FirstCommit, error) {
	first := make(map[string]FirstCommit, len(refs))

	for _, ref := range refs {
		commits, err := r.git.Log(ctx, gitx.LogSpec{Rev: trunk + ".." + ref, Reverse: true})
		if err != nil {
			return nil, err
		}

		// A branch with nothing ahead of trunk has no first commit. It cannot
		// be a claim either — a claim is a file saying something trunk's does
		// not — so this is a lookup with no answer rather than a failure.
		if len(commits) == 0 {
			continue
		}

		first[ref] = FirstCommit{
			Ref:     ref,
			OID:     commits[0].OID,
			Author:  commits[0].Author,
			Subject: commits[0].Subject,
		}
	}

	return first, nil
}
