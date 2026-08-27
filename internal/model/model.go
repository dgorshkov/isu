// Package model derives what is true about an issue from what the refs say
// about it.
//
// Nothing here is stored. An issue file carries a `state:`, which is one ref's
// claim about one issue; a *status* is what the repository as a whole says, and
// it exists only for as long as it takes to render it. That is the product: the
// tracker has no database because the refs are the database.
//
// This package is a pure function over what internal/repo loaded, and there are
// no exceptions to that later. Trunk, every other ref, the trunk history index
// and the first commit on each claiming branch are all handed in. If a future
// status needs something else, the loader grows and this package is handed the
// result — a derivation that could run a git process would run one per issue
// the first time somebody was in a hurry.
package model

import (
	"sort"
	"time"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// Status is what the repository says about an issue. It is derived from `state`
// read across refs and is never written down.
type Status string

// The derived statuses, in the precedence order of PLAN.md's table: several of
// them overlap, and the first match wins.
const (
	// StatusDone is a terminal trunk state, and it beats every claim: a
	// finished issue reads done whether or not the branch that claimed it was
	// tidied up.
	StatusDone Status = "done"
	// StatusDropped is trunk saying the issue will not be fixed. Why is in the
	// file, in `resolution`.
	StatusDropped Status = "dropped"
	// StatusAwaitingTriage is a folder on a branch that trunk has never seen —
	// a report nobody has accepted yet.
	StatusAwaitingTriage Status = "awaiting triage"
	// StatusInProgress is somebody's branch saying resolved where trunk still
	// says open. That is the claim itself, and the only source of this status.
	StatusInProgress Status = "in progress"
	// StatusReopened is open on trunk now and resolved at an earlier trunk
	// commit. It loses to in progress, because somebody actively re-fixing an
	// issue needs to show as worked rather than as merely broken again — and it
	// survives as an annotation on whatever status wins.
	StatusReopened Status = "reopened"
	// StatusOpen is on trunk, open, and nobody claiming it.
	StatusOpen Status = "open"
)

// Statuses lists every status in that precedence order.
var Statuses = []Status{
	StatusDone, StatusDropped, StatusAwaitingTriage,
	StatusInProgress, StatusReopened, StatusOpen,
}

// Terminal reports whether the status is one an issue does not come back from
// on its own.
func (s Status) Terminal() bool { return s == StatusDone || s == StatusDropped }

// Item is one issue as the board reads it.
type Item struct {
	// ID is the folder name, which is what every parent and blocked_by in the
	// repository points at.
	ID string
	// Issue is trunk's copy where there is one, because trunk is where state is
	// true. For an issue trunk has never seen it is the copy on the first ref
	// that carries it, in ref-name order.
	Issue *issue.Issue
	// Status is the derived status.
	Status Status
	// OnTrunk says the issue's folder exists at trunk.
	OnTrunk bool
	// Reopened says trunk resolved this issue once and says open now. It is an
	// annotation rather than only a status, so that the fact survives losing
	// the precedence contest to in progress.
	Reopened bool
	// Claims are the branches claiming this issue, in ref-name order.
	Claims []Claim
	// Elsewhere names the non-trunk refs whose copy of this issue differs from
	// trunk's: the branch an untriaged report is on, the branches claiming it,
	// a branch that edited the file. An issue no branch has touched has none,
	// which is what keeps two hundred branches from costing every issue a list.
	Elsewhere []string
	// Epic is the fold over this issue's children, and is set only on an issue
	// of type epic.
	Epic *Epic
}

// Board is every issue the repository holds, derived.
type Board struct {
	// Items are the issues, keyed by id.
	Items map[string]*Item
	// Children indexes every issue by the parent it names — including a parent
	// that is not an epic and a parent that does not exist, both of which are
	// M5-S2's to report and neither of which this package drops.
	Children map[string][]string

	// refs are the non-trunk refs this was derived from, in name order. It is
	// what Elsewhere and Claims are drawn from, and keeping it here means a
	// caller rendering a board does not also have to hold the loaded one.
	refs []string
}

// Get returns one issue.
func (b *Board) Get(id string) (*Item, bool) {
	i, ok := b.Items[id]

	return i, ok
}

// IDs lists the issues in id order, so that two runs over the same repository
// render the same way.
func (b *Board) IDs() []string {
	ids := make([]string, 0, len(b.Items))
	for id := range b.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	return ids
}

// Names lists the non-trunk refs the board was derived from, in name order.
func (b *Board) Names() []string { return b.refs }

// Input is everything derivation reads. Every field is loaded by internal/repo.
type Input struct {
	// Loaded is trunk and every other ref, from repo.LoadBoard. It is required.
	Loaded *repo.Board
	// History is what trunk has said about each issue over time, from
	// repo.LoadHistory. It is the only source of `reopened`.
	History repo.History
	// Claims is the first commit on each claiming branch, from
	// repo.LoadFirstCommits, keyed by ref name — usually over the refs
	// ClaimRefs named. A ref missing from it still claims: the claim is what
	// the file says, and this only names who made it and when.
	Claims map[string]repo.FirstCommit
	// Config is the repository's .isu.yml, which is where stale_days lives.
	// Pass config.Default() rather than the zero value: a claim is stale once
	// it is older than StaleDays, and zero days makes every claim stale.
	Config config.Config
	// Now is what claim age and staleness are measured against. The zero value
	// means the clock, so that only the tests have to say.
	Now time.Time
}

// Derive reads every ref and says what is true.
func Derive(in Input) *Board {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	d := &deriver{in: in, now: now, board: &Board{
		Items:    map[string]*Item{},
		Children: map[string][]string{},
		refs:     in.Loaded.Names(),
	}}

	d.collect()
	d.index()
	d.status()

	return d.board
}

// deriver is one derivation. It exists so that the phases below can be read in
// order rather than threaded through six arguments each.
type deriver struct {
	in    Input
	now   time.Time
	board *Board
	// folding names the epics part-way through their rollup, which is how a
	// parent cycle is caught rather than recursed into; folded names the ones
	// whose status is final.
	folding map[string]bool
	folded  map[string]bool
}

// collect turns trunk and the refs into one item per issue.
//
// Trunk goes in first and its copy wins, because trunk is where state is true.
// The refs then contribute only the files they changed — which is the whole
// reason repo.Board carries that list, and the difference between a board that
// costs the repository once and one that costs it once per branch.
func (d *deriver) collect() {
	for id, i := range d.in.Loaded.Trunk.Issues {
		d.board.Items[id] = &Item{ID: id, Issue: i, OnTrunk: true}
	}

	for _, ref := range d.board.refs {
		set := d.in.Loaded.Refs[ref]

		for _, id := range d.in.Loaded.Changed[ref] {
			// Not there means this ref deleted the folder, or could not read
			// it. Either way the ref is not somewhere this issue can be read
			// and trunk's copy stands: deriving from a branch's absence would
			// let one branch take an issue off everybody's board.
			at, ok := set.Get(id)
			if !ok {
				continue
			}

			item, known := d.board.Items[id]
			if !known {
				item = &Item{ID: id, Issue: at}
				d.board.Items[id] = item
			}

			item.Elsewhere = append(item.Elsewhere, ref)

			if item.OnTrunk && claimed(item.Issue, at) {
				item.Claims = append(item.Claims, d.claim(ref))
			}
		}
	}
}

// status walks the table in PLAN.md section 1, in its order, because the rows
// overlap and the first match wins. The epics are folded afterwards, over the
// statuses this leaves behind.
func (d *deriver) status() {
	for _, item := range d.board.Items {
		item.Reopened = Reopened(d.in.History[item.ID])
		item.Status = status(item)
	}

	d.rollUp()
}

func status(i *Item) Status {
	switch {
	case i.OnTrunk && i.Issue.State == issue.StateResolved:
		return StatusDone
	case i.OnTrunk && i.Issue.State == issue.StateDropped:
		return StatusDropped
	case !i.OnTrunk:
		// An epic reported on a branch lands here too, and should: a folder
		// trunk has never seen is a report awaiting triage whatever type it
		// declares, and folding an untriaged epic's children would answer a
		// question nobody asked while hiding the one that matters.
		return StatusAwaitingTriage
	case len(i.Claims) > 0:
		return StatusInProgress
	case i.Reopened:
		return StatusReopened
	default:
		// Everything else on trunk — an epic, whose status the rollup replaces
		// this with, and an issue whose state is missing or misspelt, which is
		// `isu check`'s to report. The board still has to render that one, and
		// open is the least surprising thing it can say.
		return StatusOpen
	}
}
