package github

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/issue"
)

// index is what has to be known about the whole import before any one issue can
// be mapped: which milestones have children, and how the sub-issue trees hang
// together.
type index struct {
	// present is every issue number being imported, which is what makes a
	// milestone empty and a dependency external.
	present map[int]bool
	// milestones are the ones an imported issue belongs to, by number. A
	// milestone with no imported children is not here and is not written:
	// M5-S2 makes an empty epic a check failure, and `--state open` empties
	// every milestone whose issues are all closed.
	milestones map[int]*Milestone
	// created is the earliest creation date among a milestone's children,
	// which is what an epic is dated by when the milestone itself carries no
	// date of its own.
	created map[int]time.Time
	// parent and children are the sub-issue hierarchy. isu has one `parent`,
	// the milestone is spending it, and a GitHub tree is eight levels deep —
	// so this is recorded whole and modelled not at all.
	parent   map[int]int
	children map[int][]int
	// deepest and sized are what the dry run reports about that hierarchy.
	deepest int
	sized   int
}

func newIndex(issues []Issue) *index {
	ix := &index{
		present:    map[int]bool{},
		milestones: map[int]*Milestone{},
		created:    map[int]time.Time{},
		parent:     map[int]int{},
		children:   map[int][]int{},
	}

	for _, in := range issues {
		ix.present[in.Number] = true
	}

	for _, in := range issues {
		ix.addMilestone(in)

		for _, child := range in.SubIssues {
			if ix.present[child] && child != in.Number {
				ix.parent[child] = in.Number
				ix.children[in.Number] = append(ix.children[in.Number], child)
			}
		}
	}

	ix.measure()

	return ix
}

func (ix *index) addMilestone(in Issue) {
	if in.Milestone == nil {
		return
	}

	number := in.Milestone.Number

	// The first copy wins. The REST list embeds the whole milestone on every
	// issue in it, so the copies are the same object repeated; a dump somebody
	// assembled by hand may have thinned the later ones, and overwriting would
	// then lose the description the first one carried.
	if _, known := ix.milestones[number]; !known {
		ix.milestones[number] = in.Milestone
	}

	if was, ok := ix.created[number]; !ok || in.CreatedAt.Before(was) {
		ix.created[number] = in.CreatedAt
	}
}

// measure records how deep and how large the sub-issue hierarchy is, which is
// the fact the dry run reports about the thing v1.0.0 declines to model.
func (ix *index) measure() {
	for number := range ix.children {
		ix.sized++

		if _, nested := ix.parent[number]; nested {
			continue
		}

		if depth := ix.depth(number, 1); depth > ix.deepest {
			ix.deepest = depth
		}
	}

	for child := range ix.parent {
		if _, isParent := ix.children[child]; !isParent {
			ix.sized++
		}
	}
}

// depth is how many levels hang below one issue, itself included. A cycle
// cannot happen — GitHub refuses one — but a dump somebody edited can, so the
// walk carries its own bound rather than trusting the shape.
func (ix *index) depth(number, level int) int {
	if level > maxNesting {
		return level
	}

	deepest := level

	for _, child := range ix.children[number] {
		if below := ix.depth(child, level+1); below > deepest {
			deepest = below
		}
	}

	return deepest
}

// maxNesting is GitHub's own limit on sub-issue nesting, and here it is the
// bound that stops a hand-edited dump walking forever.
const maxNesting = 8

// wanted reports whether a milestone has imported children.
func (ix *index) wanted(number int) bool { return ix.milestones[number] != nil }

// key is the source key a milestone is imported under.
//
// Milestones are numbered from their own sequence, so `#M3` and `#3` are
// different issues — `ISU-M3` and `ISU-3` — and must not collide.
func (ix *index) key(number int) string {
	return "#" + MilestoneMarker + strconv.Itoa(number)
}

// hierarchy is the whole sub-issue tree rooted at one issue, or nil when the
// issue has no children or is somebody else's child.
func (ix *index) hierarchy(number int) any {
	if _, nested := ix.parent[number]; nested {
		return nil
	}

	return ix.branch(number, 1)
}

func (ix *index) branch(number, level int) any {
	children := ix.children[number]
	if len(children) == 0 || level > maxNesting {
		return nil
	}

	sort.Ints(children)

	out := make([]any, 0, len(children))

	for _, child := range children {
		node := map[string]any{"key": "#" + strconv.Itoa(child)}
		if below := ix.branch(child, level+1); below != nil {
			node["sub_issues"] = below
		}

		out = append(out, node)
	}

	return out
}

// parentOf is the issue this one is a sub-issue of.
func (ix *index) parentOf(number int) (int, bool) {
	parent, ok := ix.parent[number]

	return parent, ok
}

// epics is one item per milestone that has imported children.
//
// A milestone is a named container whose progress is a fold over the issues in
// it, which is exactly what an isu epic is, and an issue belongs to at most one
// — so it lands in the single `parent` field with no collapse to design. The
// epic is written without `state:` like every epic, the milestone's description
// becomes its body, and its own open/closed state and due date go to
// source.yml, because here the fold is what decides.
func (ix *index) epics(repository string) []importer.Item {
	numbers := make([]int, 0, len(ix.milestones))
	for number := range ix.milestones {
		numbers = append(numbers, number)
	}

	sort.Ints(numbers)

	out := make([]importer.Item, 0, len(numbers))

	for _, number := range numbers {
		m := ix.milestones[number]
		key := ix.key(number)

		item := importer.Item{
			Key:     key,
			Ref:     repository + key,
			URL:     fmt.Sprintf("https://github.com/%s/milestone/%d", repository, number),
			Title:   m.Title,
			Type:    issue.TypeEpic,
			Epic:    true,
			Created: ix.epicCreated(number, m),
			Body:    m.Description,
			Extra: map[string]any{
				"milestone_number": number,
				"milestone_state":  m.State,
			},
		}

		if m.Creator != nil {
			item.Owner = m.Creator.Login
		}

		if m.DueOn != nil {
			item.Extra["milestone_due_on"] = m.DueOn.UTC().Format(time.DateOnly)
		}

		out = append(out, item)
	}

	return out
}

// epicCreated dates the epic: the milestone's own date where it has one, and
// the earliest of its children's otherwise. `created` is required by the
// schema, and a milestone read from `gh` carries no creation date at all.
func (ix *index) epicCreated(number int, m *Milestone) time.Time {
	if !m.CreatedAt.IsZero() {
		return m.CreatedAt
	}

	return ix.created[number]
}
