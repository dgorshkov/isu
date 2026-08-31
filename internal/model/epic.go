package model

import "github.com/dgorshkov/isu/internal/issue"

// Epic is what an epic's children add up to.
//
// Epic-ness is declared in the file rather than inferred from who points at it,
// which is what lets one issue be validated without loading another — and what
// stops a stranger's pull request from invalidating a file you own by adding a
// `parent:` line to a third file. So this is set from `type: epic` and never
// from the index below.
type Epic struct {
	// Children are the ids of the issues naming this epic as their parent, in
	// id order.
	Children []string
	// Cycle says the rollup met this epic again while folding it. Its children
	// still produced a status; what they could not produce is a status that
	// means anything, and `isu check` is where that gets reported.
	Cycle bool
}

// Empty reports whether the epic has no children. That is a check failure
// rather than a status: an epic nobody has filled in is a mistake, and the
// board still has to render it in the meantime.
//
// A nil epic is empty rather than a panic. Item.Epic is nil on every issue that
// is not one, so a renderer walking the board and asking each item what its
// children add up to is asking this of nil far more often than not — and the
// obvious spelling of that walk should answer, not crash.
func (e *Epic) Empty() bool { return e == nil || len(e.Children) == 0 }

// index maps every issue to the parent it names, once.
//
// Once is the whole point. Scanning the board for each epic's children is five
// thousand issues a thousand times over on the fixture M3-S2 is measured
// against; one pass and a map is the same answer for the price of the pass.
//
// A parent that is not an epic, and a parent that does not exist, are both kept
// here rather than dropped. Both are M5-S2's to report, and a derivation that
// quietly forgets them leaves the checker with nothing to report.
// An issue with no readable file anywhere names no parent and is no epic. It
// is on the board because trunk carries the folder, and it is skipped here
// because there is nothing in it to read, not because it does not count.
func (d *deriver) index() {
	for _, id := range d.ids {
		if item := d.board.Items[id]; item.Issue != nil && item.Issue.Parent != "" {
			d.board.Children[item.Issue.Parent] = append(d.board.Children[item.Issue.Parent], id)
		}
	}

	for id, item := range d.board.Items {
		if item.Issue != nil && item.Issue.Type == issue.TypeEpic {
			item.Epic = &Epic{Children: d.board.Children[id]}
		}
	}
}

// rollUp folds every epic's children into its status.
//
// All children terminal folds to done, unless every one of them was dropped, in
// which case the epic is dropped too. Anything else is open: an epic is not
// finished while any part of it is unfinished, and there is no `in progress`
// epic because an epic has no state to claim.
func (d *deriver) rollUp() {
	d.folding = make(map[string]foldState, len(d.board.Children))

	for _, id := range d.ids {
		if d.board.Items[id].Epic != nil {
			d.fold(id)
		}
	}
}

// foldState is where an epic is in the walk: absent means not started, and the
// two below are the rest of it. One map rather than two, because "part-way
// through" and "finished" are the same fact about one epic and holding them
// apart lets them disagree.
type foldState uint8

const (
	folding foldState = iota + 1
	folded
)

// fold gives one issue its status, folding an epic's children first.
//
// The three states an epic can be in during the walk are what makes this safe
// on a cycle: not started, part-way through, and finished. Meeting one that is
// part-way through means the parent chain came back round to where it started,
// and the answer is a value — open, the same as any epic with unfinished
// children — rather than one more frame on the stack. An epic that is its own
// parent is the one-node version of that, and it is the case that blew a stack
// during prototyping.
func (d *deriver) fold(id string) Status {
	item := d.board.Items[id]

	// Not an epic: it already has its own status, from its own state. Not on
	// trunk: it is a report awaiting triage whatever it declares, and the table
	// has already said so — an epic is folded, not exempted.
	if item.Epic == nil || !item.OnTrunk {
		return item.Status
	}

	switch d.folding[id] {
	case folded:
		return item.Status
	case folding:
		item.Epic.Cycle = true

		return StatusOpen
	}

	d.folding[id] = folding
	item.Status = d.foldChildren(item.Epic.Children)
	d.folding[id] = folded

	return item.Status
}

// foldChildren is the fold itself.
//
// Every child is folded, including the ones after the first unfinished one.
// Stopping early would be the same answer for this epic and a different one for
// the board: an epic further down that nothing else points at would keep
// whatever status it had before the rollup, decided by where in the walk it
// happened to sit.
func (d *deriver) foldChildren(children []string) Status {
	if len(children) == 0 {
		// An epic with no children. Nothing folds to nothing, so the status is
		// the one that says the least, and Epic.Empty says the rest.
		return StatusOpen
	}

	var terminal, dropped int

	for _, child := range children {
		switch status := d.fold(child); {
		case status == StatusDropped:
			terminal++
			dropped++
		case status.Terminal():
			terminal++
		}
	}

	switch {
	case terminal < len(children):
		return StatusOpen
	case dropped == len(children):
		return StatusDropped
	default:
		return StatusDone
	}
}
