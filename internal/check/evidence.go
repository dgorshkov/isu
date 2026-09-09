package check

import (
	"fmt"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

func init() {
	Checks.MustRegister(evidenceCheck{})
}

// evidenceCheck is the type table in the data model, read as a rule about a
// diff: resolving a bug, a story or a chore requires a change outside issues/,
// and resolving a spike requires the answer written down beside it.
//
// **This is the rule that makes a claim a claim.** `isu claim` writes
// `state: resolved` on the claiming branch before any work starts — that is the
// whole compare-and-swap, and it is why a claimed issue reads `in progress` — so
// a branch that claimed something and did nothing looks, in the file, exactly
// like a branch that resolved it. What separates them is not the field, which is
// identical, but the diff beside it. A branch carrying nothing but a claim fails
// this check, and that is the check doing its job.
//
// It reads the branch and not the board, so an issue no commit on this branch
// touched is not its business. A pull request is answerable for what it changed.
type evidenceCheck struct{}

func (evidenceCheck) Name() string { return "evidence" }
func (evidenceCheck) Scope() Scope { return ScopeBranch }
func (evidenceCheck) Describe() string {
	return "resolving something takes work beside it, and a drop takes a reason"
}

func (evidenceCheck) Run(in Input) []Finding {
	if in.Branch == nil {
		return nil
	}

	var out []Finding

	for _, change := range in.Branch.Issues {
		if !resolving(change) {
			continue
		}

		// Terminal is resolved or dropped, and resolving() has already said it
		// is one of them.
		if change.After.State == issue.StateResolved {
			out = append(out, resolutionFindings(in, change)...)
		} else {
			out = append(out, dropFindings(in, change)...)
		}
	}

	return out
}

// resolving reports whether this branch is the one taking an issue to a
// terminal state.
//
// Both halves matter. An issue the branch did not find open here was not marked
// done by this branch — and an issue the branch *created* in a terminal state
// was never open in this repository at all, which is what an import is: M7
// writes thousands of issues another tracker closed years ago, on a branch that
// changes nothing outside issues/ because there is nothing else to change. A
// rule that demanded code for those would make the importer unimplementable
// while catching nobody, since nothing anybody was tracking was marked done.
func resolving(change repo.IssueChange) bool {
	return change.After != nil &&
		change.Before != nil &&
		!change.Before.State.Terminal() &&
		change.After.State.Terminal()
}

// resolutionFindings is what an issue being resolved has to show for it.
func resolutionFindings(in Input, change repo.IssueChange) []Finding {
	if change.After.Type == issue.TypeSpike {
		// A spike is answered rather than fixed, so the artifact is the
		// deliverable: the answer written down beside the question. A comment
		// is not one — it is a remark in a thread, and the thing a spike owes
		// the next person is a file they can open.
		if len(in.Files.Attachments(change.ID)) > 0 {
			return nil
		}

		return []Finding{{
			Severity: SeverityFail, ID: change.ID, Path: change.Path,
			Message: "a spike is resolved by the answer, and this folder holds nothing " +
				"but its README: write the decision down beside the question",
		}}
	}

	if len(in.Branch.Outside(repo.IssuesDir)) > 0 {
		return nil
	}

	return []Finding{{
		Severity: SeverityFail, ID: change.ID, Path: change.Path,
		Message: fmt.Sprintf(
			"resolved on this branch, which changes nothing outside %s/: a claim "+
				"writes `state: resolved` and touches nothing else, so this is a claim "+
				"and not a resolution", repo.IssuesDir),
	}}
}

// dropFindings is what dropping an issue is allowed to carry.
//
// A drop that also changes code is a warning and not a failure. Closing a
// duplicate in the same pull request as the fix is a normal thing to do, and
// refusing it teaches people to split one review into two.
//
// The reason and the resolution a drop requires are not checked here. They are
// the schema's — `state: dropped` without them does not satisfy Validate — and
// a second rule reporting the same line twice would give a reader two things to
// fix that are one thing.
func dropFindings(in Input, change repo.IssueChange) []Finding {
	outside := in.Branch.Outside(repo.IssuesDir)
	if len(outside) == 0 {
		return nil
	}

	return []Finding{{
		Severity: SeverityWarn, ID: change.ID, Path: change.Path,
		Message: fmt.Sprintf(
			"dropped on a branch that also changes %s: that is a normal thing to do "+
				"when the fix and the duplicate travel together, and worth a second "+
				"look when they do not", outside[0]),
	}}
}
