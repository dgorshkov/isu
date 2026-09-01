package check

import (
	"fmt"
	"strings"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/gitx"
)

func init() {
	Checks.MustRegister(ownerCheck{})
}

// ownerCheck holds `owner:` still against everything that is not a person.
//
// `owner` is the human answerable for an issue. It is set at triage, and who is
// *working* on it right now is a different question that the claiming branch
// already answers — so an agent has no reason to touch this field and one very
// good reason not to: accountability that a tool can reassign is accountability
// that quietly ends up on whoever last ran the tool.
//
// A human may change it freely. That is the whole asymmetry, and it is why this
// rule reads the commit author rather than the diff alone: the same one-line
// change is fine from a person and is not fine from the thing they are
// supervising.
type ownerCheck struct{}

func (ownerCheck) Name() string { return "owner" }
func (ownerCheck) Scope() Scope { return ScopeBranch }
func (ownerCheck) Describe() string {
	return "no commit by a configured agent reassigns an issue's owner"
}

func (ownerCheck) Run(in Input) []Finding {
	if in.Branch == nil {
		return nil
	}

	var out []Finding

	for _, commit := range in.Branch.Commits {
		if !isAgent(in.Config, commit.Author) {
			continue
		}

		for _, edit := range commit.Edits {
			// A commit that created the file, or one whose result nothing can
			// decode. Neither is a reassignment: there was no owner to move.
			if edit.Before == nil || edit.After == nil {
				continue
			}

			if edit.Before.Owner == edit.After.Owner {
				continue
			}

			out = append(out, Finding{
				Severity: SeverityFail, ID: edit.ID, Path: edit.Path,
				Message: fmt.Sprintf(
					"%s is an agent and commit %s moves owner from %q to %q: owner is "+
						"the accountable human and is set at triage, so only a person "+
						"changes it",
					commit.Author.Name, shortOID(commit.OID),
					edit.Before.Owner, edit.After.Owner),
			})
		}
	}

	return out
}

// isAgent reports whether a commit author is one of the authors .isu.yml calls
// an agent.
//
// A name is matched as written and an address without regard to case, which is
// how the two are actually equal: `Claude` and `claude` are two display names
// and somebody meant something by the difference, where CLAUDE@example.com and
// claude@example.com are one mailbox.
func isAgent(cfg config.Config, who gitx.Signature) bool {
	for _, name := range cfg.Agents {
		if name == who.Name || strings.EqualFold(name, who.Email) {
			return true
		}
	}

	return false
}

// shortOID is a commit id at the length a person reads.
func shortOID(oid string) string {
	if len(oid) <= 8 {
		return oid
	}

	return oid[:8]
}
