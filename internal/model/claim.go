package model

import (
	"time"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// Claim is one branch claiming one issue.
//
// `isu claim` creates branch isu/<ID>, writes `state: resolved` into the issue
// and pushes — before any work starts, so that the loser of a race wastes
// nothing. The push is the compare-and-swap and the state flip is what makes it
// one: two claimants produce two different commits, so the second push is not a
// fast-forward and git rejects it.
//
// Which means a claim is not advisory metadata to reconcile against the
// branches. It is what the branch says, read the same way as everything else.
type Claim struct {
	// Ref is the full name of the claiming branch.
	Ref string
	// Claimant is the author of the commit that flipped the state, and empty
	// when that commit was not looked up. A claim with no claimant is still a
	// claim: what the file says is the claim, and this only names who made it.
	Claimant string
	// Email is that author's address.
	Email string
	// Commit is the commit that flipped the state — the first commit on the
	// branch, never its tip.
	Commit string
	// When is that commit's author date, which is when the claim was made.
	When time.Time
	// Age is how long ago that was, and is zero when the claim time is unknown.
	Age time.Duration
	// Stale says the claim is older than stale_days. Somebody said they were
	// doing this and then went quiet, which is the thing a board full of
	// abandoned claims stops being able to tell you.
	Stale bool
}

// Contended reports whether more than one branch is claiming this issue. Two
// people about to do the same work is what a board exists to prevent, so it has
// a name rather than a length comparison at every call site.
func (i *Item) Contended() bool { return len(i.Claims) > 1 }

// Stale reports whether any claim on this issue has gone quiet.
func (i *Item) Stale() bool {
	for _, claim := range i.Claims {
		if claim.Stale {
			return true
		}
	}

	return false
}

// ClaimRefs lists the refs claiming something, which are the refs a caller has
// to look up first commits for before deriving.
//
// It is a pure question about what is already loaded, so it is answered here;
// the lookup it feeds costs a git process per ref, so that belongs to the
// loader. Splitting them is what keeps the process count to the branches that
// actually claim rather than every branch in the repository.
func ClaimRefs(loaded *repo.Board) []string {
	var refs []string

	for _, ref := range loaded.Names() {
		set := loaded.Refs[ref]

		for _, id := range loaded.Changed[ref] {
			on, carried := set.Get(id)
			at, onTrunk := loaded.Trunk.Get(id)

			if carried && onTrunk && claimed(at, on) {
				refs = append(refs, ref)

				break
			}
		}
	}

	return refs
}

// claim is one ref's claim, annotated with whoever made it.
func (d *deriver) claim(ref string) Claim {
	claim := Claim{Ref: ref}

	first, known := d.in.Claims[ref]
	if !known {
		return claim
	}

	claim.Claimant = first.Author.Name
	claim.Email = first.Author.Email
	claim.Commit = first.OID
	claim.When = first.Author.When
	claim.Age = d.now.Sub(claim.When)
	claim.Stale = claim.Age > d.in.Config.StaleAfter()

	return claim
}

// claimed reports whether a branch's copy of an issue claims it: `state:
// resolved` where trunk says open.
//
// A branch that exists without that flip is deliberately not a claim. It is
// somebody's work on a branch, and until they say so by claiming, the board
// does not speak for them — which is also what makes `isu unclaim` mean
// something, since it flips the state back and leaves the branch standing.
func claimed(atTrunk, onBranch *issue.Issue) bool {
	return atTrunk.State == issue.StateOpen && onBranch.State == issue.StateResolved
}
