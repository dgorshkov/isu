// Package check runs isu's rules over a repository and says what is wrong with
// it.
//
// The rules themselves are not new. Almost every one of them is a sentence
// PLAN.md section 1 already wrote down — a parent names an epic, resolving
// something requires a change outside issues/, an agent never reassigns an
// owner — and until this package existed there was nowhere for those sentences
// to be enforced. `isu check` is where a pull request finds out.
//
// # What a check may read
//
// A check is a pure function of an Input, and an Input is what internal/repo
// loaded: trunk and every ref, the derived board, the files beside each issue,
// and what the branch under review proposes. **No check runs git.** That is the
// same rule internal/model lives under and it is here for the same reason: a
// check that could spawn a process would spawn one per issue the first time
// somebody was in a hurry, and the read path in PLAN.md §0 exists precisely so
// that nothing has to.
//
// # Two scopes, and why a hook needs them
//
// Checks divide into what is true of the repository as it stands and what is
// true of what a branch proposes. The division is not decoration: the
// pre-commit hook in M5-S6 can only ask the first kind. At pre-commit time the
// change being committed is not a commit yet, so the branch questions would be
// answered from the commits already on it — and on a claiming branch that
// answer is "you resolved an issue and wrote no code", which would block the
// very commit that writes the code. So the hook runs ScopeTree and the pipeline
// runs everything.
package check

import (
	"fmt"
	"sort"
	"time"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// Severity is how much a finding matters.
type Severity string

const (
	// SeverityFail is a rule the repository breaks, and it is what makes `isu
	// check` exit non-zero.
	SeverityFail Severity = "fail"
	// SeverityWarn is something worth saying that nobody has done wrong.
	// Contention is the shape of it: two people about to do the same work is
	// worth a sentence and is not a reason to fail a pull request.
	SeverityWarn Severity = "warn"
)

// Severities lists them in the order a report renders, which is the order they
// matter in.
var Severities = []Severity{SeverityFail, SeverityWarn}

// rank is where a severity sorts. A severity nothing registered sorts last
// rather than first, so that a typo cannot quietly outrank a failure.
func (s Severity) rank() int {
	for i, known := range Severities {
		if known == s {
			return i
		}
	}

	return len(Severities)
}

// Scope says which question a check answers: what the repository is, or what
// this branch proposes to make it.
type Scope string

const (
	// ScopeTree is a check about the repository as it stands. It needs no
	// branch and gives the same answer to everybody looking at the same refs.
	ScopeTree Scope = "tree"
	// ScopeBranch is a check about the difference between trunk and the branch
	// under review. With no branch there is nothing for it to say.
	ScopeBranch Scope = "branch"
)

// Scopes lists them, which is what `isu check --scope` accepts.
var Scopes = []Scope{ScopeTree, ScopeBranch}

// Finding is one thing a check found.
//
// The check's own name is stamped on by the registry rather than written by
// each check, so that a check cannot disagree with the name it was registered
// under — and so that adding one stays the one file and one registry line
// PLAN.md asks for.
type Finding struct {
	// Check is the name of the check that found it.
	Check string
	// Severity is fail or warn.
	Severity Severity
	// ID is the issue this is about, or empty when it is about the repository.
	ID string
	// Path is the file it is about, relative to the repository root, or empty.
	Path string
	// Message says what is wrong, in one sentence and without repeating the id.
	Message string
}

// Fetch is how old the refs a check read are.
//
// Every derived status in this product is a statement about refs, so a check
// that reports contention is only as true as the last fetch. That is not a
// footnote: two engineers with different fetch ages see different contention,
// and a warning that cannot say which one it is speaking from is a warning they
// will argue about.
type Fetch struct {
	// Remote says there are remote-tracking refs at all.
	Remote bool
	// Newest is the date of the newest of them.
	Newest time.Time
}

// Age is how long ago the newest remote-tracking ref was written.
func (f Fetch) Age(now time.Time) time.Duration {
	if !f.Remote || f.Newest.IsZero() {
		return 0
	}

	return now.Sub(f.Newest)
}

// Caveat is the clause a warning about refs carries when the refs it read
// cannot be trusted to be current, and the empty string when they can.
//
// It is deliberately a clause and not a finding of its own: a report that said
// "your refs are old" once, somewhere else, would leave every warning above it
// reading as a confident statement about the whole team.
func (f Fetch) Caveat(cfg config.Config, now time.Time) string {
	if !f.Remote {
		return "; these are local branches only, so nobody else's work is in this answer"
	}

	age := f.Age(now)
	if age <= cfg.FetchWarnAfter() {
		return ""
	}

	return fmt.Sprintf(
		"; the newest remote ref is %d hours old, so this may be out of date — run with --fetch",
		int(age.Hours()))
}

// Input is everything a check may read. Every field is loaded by internal/repo
// or derived by internal/model, and none of it is a git process.
type Input struct {
	// Trunk is the ref this run treats as trunk, for messages that have to say
	// what they were compared against.
	Trunk string
	// Head is the ref under review — the branch a pull request would carry, or
	// HEAD where a pipeline checked out no branch. It is empty when there is
	// nothing beside trunk to look at.
	Head string
	// Board is the derived board: statuses, claims, epics.
	Board *model.Board
	// Loaded is trunk and every ref it was derived from, which is where a check
	// looks when it needs a branch's own copy of a file rather than the one
	// derivation chose.
	Loaded *repo.Board
	// Files are the files beside each issue's README at the ref under review,
	// keyed by issue id. Attachments have a size cap and a spike's resolution
	// needs one of them to exist, and neither question can be asked of the
	// README alone.
	Files repo.Files
	// Branch is what Head proposes over trunk: its commits, their authors, what
	// each of them did to which issue, and the paths that differ. It is nil
	// when there is no branch, which is what makes every ScopeBranch check a
	// no-op rather than a special case.
	Branch *repo.Branch
	// Config is the repository's .isu.yml.
	Config config.Config
	// Fetch is how old the refs above are, which is how old every answer drawn
	// from them is.
	Fetch Fetch
	// Now is what ages are measured against. The zero value means the clock.
	Now time.Time
}

// When is the moment this run is measured against.
func (in Input) When() time.Time {
	if in.Now.IsZero() {
		return time.Now()
	}

	return in.Now
}

// Check is one rule.
//
// Adding one is one file and one registry line, which is the whole design brief
// for this package: a rule nobody can add cheaply is a rule that gets written
// into a reviewer's habits instead.
type Check interface {
	// Name is what the report calls it. It is stable: a pipeline that greps for
	// one is reading a contract.
	Name() string
	// Scope says whether it reads the repository or the branch.
	Scope() Scope
	// Describe is one line saying what it enforces, for `isu check --list`.
	Describe() string
	// Run reports everything it found. It never runs git and never fails: a
	// check that cannot answer has found nothing.
	Run(Input) []Finding
}

// Registry is the set of checks a run has.
//
// Checks is the default one, which every check file registers itself with in an
// init function. A test builds its own rather than reaching into that, so a
// test's fixtures cannot leak into the product's list.
type Registry struct {
	checks map[string]Check
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{checks: map[string]Check{}} }

// Checks is the registry `isu check` runs.
var Checks = NewRegistry()

// Len is how many checks are registered.
func (r *Registry) Len() int { return len(r.checks) }

// Register adds a check, refusing a name that is empty or already taken.
//
// A duplicate name is refused rather than overwritten because the name is what
// a report and a pipeline both key on: two checks answering to one name is a
// report where half the findings are attributed to the wrong rule.
func (r *Registry) Register(c Check) error {
	name := c.Name()
	if name == "" {
		return fmt.Errorf("a check needs a name: %T has none", c)
	}

	if _, taken := r.checks[name]; taken {
		return fmt.Errorf("a check called %q is already registered", name)
	}

	r.checks[name] = c

	return nil
}

// MustRegister adds a check and panics if it cannot, which is what an init
// function does with a mistake nobody can recover from.
func (r *Registry) MustRegister(c Check) {
	if err := r.Register(c); err != nil {
		panic("check: " + err.Error())
	}
}

// All lists the registered checks in name order.
//
// Name order rather than registration order, so that where a registry line sits
// in a file cannot change the output of a pipeline.
func (r *Registry) All() []Check {
	out := make([]Check, 0, len(r.checks))
	for _, c := range r.checks {
		out = append(out, c)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })

	return out
}

// Run runs the checks whose scope was asked for, or all of them when none was.
func (r *Registry) Run(in Input, scopes ...Scope) Report {
	report := Report{Checks: []string{}, Findings: []Finding{}}

	for _, c := range r.All() {
		if !wanted(c.Scope(), scopes) {
			continue
		}

		report.Checks = append(report.Checks, c.Name())

		for _, f := range c.Run(in) {
			f.Check = c.Name()
			report.Findings = append(report.Findings, f)
		}
	}

	report.sort()

	return report
}

// wanted reports whether a scope was asked for. Asking for none asks for all of
// them: `isu check` with no flag is the whole suite.
func wanted(scope Scope, scopes []Scope) bool {
	if len(scopes) == 0 {
		return true
	}

	for _, s := range scopes {
		if s == scope {
			return true
		}
	}

	return false
}

// Report is what one run found.
type Report struct {
	// Checks are the names of the checks that ran, in name order.
	Checks []string
	// Findings are what they found, most severe first.
	Findings []Finding
}

// Count is how many findings of one severity there are.
func (r Report) Count(s Severity) int {
	n := 0

	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}

	return n
}

// OK reports whether the repository passes: warnings are not failures, and a
// pull request that only has something worth saying about it still merges.
func (r Report) OK() bool { return r.Count(SeverityFail) == 0 }

// sort puts the findings in the one order every run of the same checks over the
// same refs produces: failures first, then by check, then by issue.
//
// Determinism here is not tidiness. A pipeline's output is diffed by whoever is
// trying to work out what their commit changed, and a report that shuffles
// itself makes every run look like a new problem.
func (r *Report) sort() {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]

		switch {
		case a.Severity != b.Severity:
			return a.Severity.rank() < b.Severity.rank()
		case a.Check != b.Check:
			return a.Check < b.Check
		case a.ID != b.ID:
			return a.ID < b.ID
		case a.Path != b.Path:
			return a.Path < b.Path
		default:
			return a.Message < b.Message
		}
	})
}
