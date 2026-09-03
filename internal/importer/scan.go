package importer

import (
	"strings"
	"time"

	"github.com/dgorshkov/isu/internal/gitx"
)

// Tier says which source recovered a link between an issue and the commit that
// resolved it. Recording it matters because the tiers are not equally
// trustworthy, and an import that cannot say how it knows something cannot be
// audited.
//
// Issues resolved after the switch to isu carry an `Isu-Resolves:` trailer and
// need none of this — internal/model reads that one. These tiers exist for the
// years of history that predate it.
type Tier int

// The tiers, weakest first, so that a stronger one is simply a larger number
// and "record the stronger of two" is a comparison.
const (
	// TierNone is no link at all, which is the right answer far more often
	// than a guess is.
	TierNone Tier = iota
	// TierSquashSubject is a key in a commit subject. Under a squash merge the
	// subject is whatever the forge composed — often a pull request title — so
	// this is the weakest thing that is still evidence.
	TierSquashSubject
	// TierMergeBranch is a key in the branch name a merge commit recorded.
	// Branch names are not composed by a forge setting, which is what makes
	// this stronger than the subject.
	TierMergeBranch
	// TierMessage is a key somebody wrote in the body of a commit message.
	// Nobody types an issue key into a commit body by accident.
	TierMessage
	// TierClosing is the source's own record of the pull request or commit
	// that closed the issue. It is not recovered from history at all — GitHub
	// already stores it — so it outranks everything above.
	TierClosing
)

// tierNames is what each tier is called in a report and in --json.
var tierNames = map[Tier]string{
	TierNone:          "none",
	TierSquashSubject: "squash subject",
	TierMergeBranch:   "merge branch name",
	TierMessage:       "commit message",
	TierClosing:       "closing pull request",
}

func (t Tier) String() string {
	if name, ok := tierNames[t]; ok {
		return name
	}

	return "unknown"
}

// Link is the commit that resolved one issue, and how that was established.
type Link struct {
	// ID is the issue, as an isu id rather than as a source key.
	ID string
	// Commit is the commit that resolved it, or the pull request reference
	// when the tier is TierClosing.
	Commit string
	// Tier says which source produced the link.
	Tier Tier
	// When is the commit's committer date, and the zero time for a link that
	// did not come from a commit.
	When time.Time
}

// Evidence is the recorder: the strongest link known for each issue.
type Evidence struct {
	links map[string]Link
}

// NewEvidence returns an empty recorder.
func NewEvidence() *Evidence { return &Evidence{links: map[string]Link{}} }

// Record keeps a link when it is stronger than what is already known, and
// reports whether it did.
//
// Equal tiers keep the first, which is the oldest commit in a reverse walk: an
// issue named by two commits at the same tier was resolved by the first of
// them and mentioned again by the second.
func (e *Evidence) Record(l Link) bool {
	if known, ok := e.links[l.ID]; ok && known.Tier >= l.Tier {
		return false
	}

	e.links[l.ID] = l

	return true
}

// Link is what is known about one issue.
func (e *Evidence) Link(id string) (Link, bool) {
	l, ok := e.links[id]

	return l, ok
}

// Len is how many issues have a link at all.
func (e *Evidence) Len() int { return len(e.links) }

// Counts is how many issues were linked at each tier.
func (e *Evidence) Counts() map[Tier]int {
	out := map[Tier]int{}
	for _, l := range e.links {
		out[l.Tier]++
	}

	return out
}

// Scan reads a repository's history for links between commits and the issues
// being imported.
//
// **Every match is checked against the import and discarded when it is not one
// of these issues**, which is the whole of this function's safety. A GitHub key
// is `#1234` and it is ambiguous in a way `PROJ-1234` never was: issues and
// pull requests are numbered from one sequence, so `Merge pull request #456
// from …` names a pull request, a squash subject ending `(#456)` almost always
// does too, and `#1234` turns up in prose about nothing at all. That is why
// this cannot run before the issue list is in hand.
//
// The commits are expected oldest first, so that two commits naming one issue
// at the same tier resolve to the first of them.
func Scan(e *Evidence, commits []gitx.Commit, keys func(string) []string, m *Mapping) {
	for _, c := range commits {
		record(e, c, keys(c.Body), TierMessage, m)

		// A merge commit's subject is git's own composition, not anybody's
		// sentence: the number in `Merge pull request #456 from alice/fix` is
		// the pull request by construction. So on a merge the subject is read
		// for the branch name it recorded and for nothing else.
		if branch := mergeBranch(c); branch != "" {
			record(e, c, keys(branch), TierMergeBranch, m)
			continue
		}

		record(e, c, keys(c.Subject), TierSquashSubject, m)
	}
}

// record files every key that is one of the issues being imported.
func record(e *Evidence, c gitx.Commit, found []string, tier Tier, m *Mapping) {
	for _, key := range found {
		id, ok := m.ID(key)
		if !ok {
			continue
		}

		e.Record(Link{ID: id, Commit: c.OID, Tier: tier, When: c.Committer.When})
	}
}

// mergeBranch is the branch name a merge commit recorded, or the empty string
// when the commit is not one or its subject does not name a branch.
//
// The two spellings are git's own and the forge's: `Merge branch 'topic'`, with
// an optional `into trunk` after it, and `Merge pull request #456 from
// alice/topic`. PLAN.md's squash-merge section is why this tier exists at all —
// a repository whose squash message is the pull request title never carries a
// trailer or a useful subject onto trunk, and the branch name is recorded by
// the merge whatever that setting says.
func mergeBranch(c gitx.Commit) string {
	if len(c.Parents) < 2 {
		return ""
	}

	subject := strings.TrimSpace(c.Subject)

	if _, from, ok := strings.Cut(subject, " from "); ok &&
		strings.HasPrefix(subject, "Merge pull request ") {
		name, _, _ := strings.Cut(strings.TrimSpace(from), " ")

		return name
	}

	if _, quoted, ok := strings.Cut(subject, "Merge branch '"); ok {
		if name, _, closed := strings.Cut(quoted, "'"); closed {
			return name
		}
	}

	return ""
}
