package model

import (
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// Reopened reports whether trunk resolved an issue once and says open now.
//
// It is a fold over the trunk history index and nothing else. `reopened` is the
// one status that cannot be answered from the current content of any ref — the
// file at trunk says `open`, exactly as it did the day the issue was reported,
// and the difference between those two is a commit that is no longer the tip.
// Walking that history here would be a git process per issue, so M2-S4 walks it
// once in the loader and this reads what it found.
//
// The states come from blob content, never from commit metadata, which is what
// makes the answer survive a squash merge: squash collapses authorship, and it
// does not touch the file.
//
// `dropped` at an earlier commit is deliberately not a reopen. The table in
// the data model names `resolved`, and the two are not the same event:
// undropping is a triage decision somebody made on purpose, where a reopen is
// the repository reporting that a fix did not hold.
func Reopened(states []repo.StateAt) bool {
	if len(states) == 0 {
		return false
	}

	latest := states[len(states)-1]
	if latest.Removed || latest.State != issue.StateOpen {
		return false
	}

	for _, at := range states[:len(states)-1] {
		if !at.Removed && at.State == issue.StateResolved {
			return true
		}
	}

	return false
}
