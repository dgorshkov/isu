package check

import (
	"fmt"
	"strings"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/model"
)

func init() {
	Checks.MustRegister(claimCheck{})
}

// claimCheck reports what the claims say about each other: two branches on one
// issue, and a claim that has gone quiet.
//
// Both are warnings. Two people about to do the same work is exactly what a
// board exists to prevent and it is not something anybody did wrong, and a
// pull request refused for it would be a pull request refused for somebody
// else's branch.
//
// # Why every one of these carries a clause about refs
//
// A claim is a branch, so both of these are statements about a ref set — and a
// ref set is only as current as the last fetch. Two engineers with different
// fetch ages see different contention, which is an argument nobody can settle
// from the output unless the output says which refs it read. Worse, the board
// reads refs/heads/, so a claim that reached this clone as
// refs/remotes/origin/isu/<ID> is not in the answer at all: that is the
// limitation M4-S8 measured and wrote down, and until the story it asks for
// lands, the honest thing for this rule to do is say so in the warning rather
// than to imply it has looked everywhere.
type claimCheck struct{}

func (claimCheck) Name() string { return "claims" }
func (claimCheck) Scope() Scope { return ScopeTree }
func (claimCheck) Describe() string {
	return "nobody is working on the same issue twice, and no claim has gone quiet"
}

func (claimCheck) Run(in Input) []Finding {
	if in.Board == nil {
		return nil
	}

	now := in.When()
	caveat := in.Fetch.Caveat(in.Config, now) + reach(in.Fetch)

	var out []Finding

	for _, id := range in.Board.IDs() {
		item, _ := in.Board.Get(id)

		if item.Contended() {
			out = append(out, Finding{
				Severity: SeverityWarn, ID: id, Path: readmePath(id),
				Message: fmt.Sprintf(
					"claimed on %d branches at once: %s%s",
					len(item.Claims), holders(item.Claims), caveat),
			})
		}

		for _, claim := range item.Claims {
			if !claim.Stale {
				continue
			}

			out = append(out, Finding{
				Severity: SeverityWarn, ID: id, Path: readmePath(id),
				Message: fmt.Sprintf(
					"claimed %d days ago on %s by %s and it is still open: %s is %d%s",
					int(claim.Age.Hours())/24, shortRef(claim.Ref), who(claim),
					config.KeyStaleDays, in.Config.StaleDays, caveat),
			})
		}
	}

	return out
}

// reach is the sentence that keeps a claim warning from claiming more than it
// looked at. See the note on claimCheck.
func reach(fetch Fetch) string {
	if !fetch.Remote {
		// The caveat already said this repository has no remote, which is a
		// stronger statement than this one.
		return ""
	}

	return "; isu reads refs/heads/, so a claim somebody else pushed and never " +
		"merged is not in this answer"
}

// holders renders the branches claiming an issue and who is on each.
func holders(claims []model.Claim) string {
	parts := make([]string, 0, len(claims))

	for _, claim := range claims {
		parts = append(parts, shortRef(claim.Ref)+" ("+who(claim)+")")
	}

	return strings.Join(parts, ", ")
}

// who is the claimant, or the honest answer when the claim's first commit was
// never looked up. A claim with no claimant is still a claim: what the file
// says is the claim, and the lookup only names who made it.
func who(claim model.Claim) string {
	if claim.Claimant == "" {
		return "someone"
	}

	return claim.Claimant
}
