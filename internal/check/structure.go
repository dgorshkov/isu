package check

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/issue"
)

// The structural rules: everything the data model says about an issue that
// can be answered without a diff.
//
// Two of them are answers to the same question asked at different distances.
// `Validate` in M1-S2 takes one issue and nothing else — that is why epic-ness
// is declared in the file rather than inferred from who points at it, and why
// `parent:` is checked for shape there and not for what it names. Here the whole
// set is loaded, so the second half of those rules can finally be enforced: the
// id resolves, and what it resolves to is an epic.

func init() {
	Checks.MustRegister(schemaCheck{})
	Checks.MustRegister(duplicateCheck{})
	Checks.MustRegister(linkCheck{})
	Checks.MustRegister(cycleCheck{})
	Checks.MustRegister(epicCheck{})
	Checks.MustRegister(attachmentCheck{})
}

// schemaCheck is the schema itself, at every ref that carries a copy.
type schemaCheck struct{}

func (schemaCheck) Name() string { return "schema" }
func (schemaCheck) Scope() Scope { return ScopeTree }
func (schemaCheck) Describe() string {
	return "every issue file parses, satisfies the schema, and sits in a folder named after its id"
}

func (schemaCheck) Run(in Input) []Finding {
	var out []Finding

	for _, copied := range in.Copies() {
		if copied.Err != nil {
			out = append(out, Finding{
				Severity: SeverityFail, ID: copied.ID, Path: copied.Path,
				Message: copied.Where("will not decode: " + copied.Err.Error()),
			})

			continue
		}

		var invalid *issue.ValidationError
		if errors.As(copied.Issue.Validate(), &invalid) {
			// One finding per problem rather than one per file: a file with
			// four things wrong is four things to fix, and a report that
			// folded them into one line would be a line nobody could tick off.
			for _, problem := range invalid.Problems {
				out = append(out, Finding{
					Severity: SeverityFail, ID: copied.ID, Path: copied.Path,
					Message: copied.Where(problem.String()),
				})
			}
		}
	}

	return out
}

// duplicateCheck is two clones that generated the same token at the same
// moment, which is the one thing standing between them and a corrupt tree.
//
// It is a question about ids that trunk has never seen, and it has to be: an id
// that is on trunk and also on a branch is one issue somebody edited, which is
// the ordinary case and the whole point of the model. Two *branches* carrying
// an id trunk has never seen, with different titles or different creation
// dates, are two issues wearing one id — and since an id is permanent from
// creation, merging both is the corruption there is no command to undo.
type duplicateCheck struct{}

func (duplicateCheck) Name() string { return "duplicates" }
func (duplicateCheck) Scope() Scope { return ScopeTree }
func (duplicateCheck) Describe() string {
	return "no two branches report different issues under one id"
}

func (duplicateCheck) Run(in Input) []Finding {
	if in.Loaded == nil {
		return nil
	}

	byID := map[string][]Copy{}
	onTrunk := map[string]bool{}

	for _, copied := range in.Copies() {
		switch {
		case copied.OnTrunk:
			// Trunk has it, so every branch carrying it is editing it — which
			// is the model working rather than two issues wearing one id.
			onTrunk[copied.ID] = true
		case copied.Issue != nil:
			byID[copied.ID] = append(byID[copied.ID], copied)
		}
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		if !onTrunk[id] {
			ids = append(ids, id)
		}
	}

	sort.Strings(ids)

	var out []Finding

	for _, id := range ids {
		found := byID[id]

		for i := 1; i < len(found); i++ {
			if sameIssue(found[0].Issue, found[i].Issue) {
				continue
			}

			out = append(out, Finding{
				Severity: SeverityFail, ID: id, Path: readmePath(id),
				Message: fmt.Sprintf(
					"%s and %s report different issues under this one id: an id is "+
						"permanent from creation, so one of them has to be filed again",
					shortRef(found[0].Ref), shortRef(found[i].Ref)),
			})

			break
		}
	}

	return out
}

// sameIssue reports whether two copies are versions of one issue rather than
// two issues.
//
// Title and creation date, and nothing else. Everything a triage edit touches —
// owner, priority, parent, blockers — is deliberately not here: a branch that
// triaged a report is not a second report, and a check that said it was would
// fire on the ordinary case rather than the corrupt one.
func sameIssue(a, b *issue.Issue) bool {
	return a.Title == b.Title && a.Created.Equal(b.Created)
}

// linkCheck is the half of `parent` and `blocked_by` that M1-S2 cannot check:
// whether the id resolves, and whether what it resolves to is an epic.
type linkCheck struct{}

func (linkCheck) Name() string { return "links" }
func (linkCheck) Scope() Scope { return ScopeTree }
func (linkCheck) Describe() string {
	return "parent and blocked_by name issues that exist, and a parent is an epic"
}

func (linkCheck) Run(in Input) []Finding {
	var out []Finding

	in.Issues(func(id string, i *issue.Issue) {
		if i.Parent != "" {
			out = append(out, parentFindings(in, id, i.Parent)...)
		}

		for _, blocker := range i.BlockedBy {
			switch {
			case blocker == id:
				out = append(out, Finding{
					Severity: SeverityFail, ID: id, Path: readmePath(id),
					Message: "blocked_by names this issue, which can never be terminal " +
						"before itself",
				})
			case !known(in, blocker):
				out = append(out, Finding{
					Severity: SeverityFail, ID: id, Path: readmePath(id),
					Message: fmt.Sprintf(
						"blocked_by names %s, and no ref isu can see has an issue by "+
							"that id", blocker),
				})
			}
		}
	})

	return out
}

func parentFindings(in Input, id, parent string) []Finding {
	if parent == id {
		return []Finding{{
			Severity: SeverityFail, ID: id, Path: readmePath(id),
			Message: "names itself as its parent",
		}}
	}

	item, ok := in.Board.Get(parent)
	if !ok {
		return []Finding{{
			Severity: SeverityFail, ID: id, Path: readmePath(id),
			Message: fmt.Sprintf(
				"parent %s does not exist on any ref isu can see", parent),
		}}
	}

	if item.Issue == nil || item.Issue.Type != issue.TypeEpic {
		return []Finding{{
			Severity: SeverityFail, ID: id, Path: readmePath(id),
			Message: fmt.Sprintf(
				"parent %s is not an epic, and only an epic has children: an epic "+
					"declares `type: epic` and no state of its own", parent),
		}}
	}

	return nil
}

// known reports whether an id resolves to something on the board.
//
// It is only ever asked from inside a walk over that board, so there is no nil
// to guard here: a guard that cannot be reached is a guard nobody has seen work.
func known(in Input, id string) bool {
	_, ok := in.Board.Get(id)

	return ok
}

// cycleCheck is a parent chain or a dependency chain that comes back round.
//
// Derivation survives both — the epic rollup treats a cycle as an epic with
// unfinished children rather than recursing into it, which is what stops a
// board from blowing a stack — but surviving is not the same as being right. An
// epic that is its own grandparent has no status anybody can act on, and a
// dependency cycle is a queue in which nothing is ever ready.
type cycleCheck struct{}

func (cycleCheck) Name() string { return "cycles" }
func (cycleCheck) Scope() Scope { return ScopeTree }
func (cycleCheck) Describe() string {
	return "no epic is its own ancestor, and nothing waits on itself in a ring"
}

func (cycleCheck) Run(in Input) []Finding {
	if in.Board == nil {
		return nil
	}

	ids := in.Board.IDs()

	var out []Finding

	for _, ring := range findCycles(ids, func(id string) []string {
		item, ok := in.Board.Get(id)
		if !ok || item.Issue == nil || item.Issue.Parent == "" {
			return nil
		}

		return []string{item.Issue.Parent}
	}) {
		out = append(out, Finding{
			Severity: SeverityFail, ID: ring[0], Path: readmePath(ring[0]),
			Message: "these issues are each other's ancestors, so none of them has a " +
				"status anybody can act on: " + strings.Join(ring, ", "),
		})
	}

	for _, ring := range findCycles(ids, func(id string) []string {
		item, ok := in.Board.Get(id)
		if !ok || item.Issue == nil {
			return nil
		}

		return item.Issue.BlockedBy
	}) {
		out = append(out, Finding{
			Severity: SeverityFail, ID: ring[0], Path: readmePath(ring[0]),
			Message: "these issues wait on each other, so none of them is ever " +
				"ready: " + strings.Join(ring, ", "),
		})
	}

	return out
}

// findCycles returns every cycle in a graph, each as its members in id order.
//
// A depth-first walk with three colours: not seen, on the stack, finished.
// Meeting something that is on the stack is a cycle, and it is the members from
// there up that are in it — the walk may have reached it through a chain that
// is not. Each cycle is reported once however many entrances it has, keyed on
// its members, because a diamond over a ring would otherwise report the ring
// twice and give a reader two things to fix that are one thing.
func findCycles(ids []string, next func(string) []string) [][]string {
	const (
		unseen = iota
		onStack
		done
	)

	state := map[string]int{}
	seen := map[string]bool{}

	var (
		stack  []string
		cycles [][]string
		walk   func(string)
	)

	walk = func(id string) {
		state[id] = onStack
		stack = append(stack, id)

		for _, to := range next(id) {
			switch state[to] {
			case unseen:
				walk(to)
			case onStack:
				// `to` is on the stack, so this walks back to it; the frames
				// below it reached the ring without being in it.
				at := len(stack) - 1
				for stack[at] != to {
					at--
				}

				ring := append([]string{}, stack[at:]...)
				if len(ring) < 2 {
					// An issue that names itself. It is a ring, and the links
					// rule says it in one sentence a reader can act on rather
					// than as a cycle of one.
					continue
				}

				sort.Strings(ring)

				if key := strings.Join(ring, ","); !seen[key] {
					seen[key] = true
					cycles = append(cycles, ring)
				}
			}
		}

		stack = stack[:len(stack)-1]
		state[id] = done
	}

	for _, id := range ids {
		if state[id] == unseen {
			walk(id)
		}
	}

	return cycles
}

// epicCheck is an epic nobody filled in.
//
// An epic's status is the fold over its children, so an epic with none folds
// over nothing. The plan calls that a check failure rather than a status, and
// this is it: the board renders such an epic as open, which is the least
// surprising thing it can say and not a thing anybody can act on.
type epicCheck struct{}

func (epicCheck) Name() string     { return "epics" }
func (epicCheck) Scope() Scope     { return ScopeTree }
func (epicCheck) Describe() string { return "every epic has at least one child" }

func (epicCheck) Run(in Input) []Finding {
	if in.Board == nil {
		return nil
	}

	var out []Finding

	for _, id := range in.Board.IDs() {
		item, _ := in.Board.Get(id)

		if item.Epic != nil && item.Epic.Empty() {
			out = append(out, Finding{
				Severity: SeverityFail, ID: id, Path: readmePath(id),
				Message: "an epic with no children folds over nothing: give it one, " +
					"or make it an issue of its own",
			})
		}
	}

	return out
}

// attachmentCheck is the per-attachment cap from .isu.yml.
//
// Issues are files in the repository, so an attachment is in everybody's clone
// for the life of the project — including the clone of the person who only
// wanted the source. The cap is the one place that is said out loud.
type attachmentCheck struct{}

func (attachmentCheck) Name() string { return "attachments" }
func (attachmentCheck) Scope() Scope { return ScopeTree }
func (attachmentCheck) Describe() string {
	return "no attachment is bigger than attachment_max_bytes"
}

func (attachmentCheck) Run(in Input) []Finding {
	// A zero cap is a configuration nobody wrote: every key in .isu.yml has a
	// default and this one's is half a megabyte. Reading it as "no attachment
	// may be larger than nothing" would fail every repository whose caller
	// built an Input without loading a config, which is the same courtesy
	// model.Derive extends to stale_days and for the same reason.
	limit := in.Config.AttachmentMaxBytes
	if limit <= 0 {
		limit = config.DefaultAttachmentMaxBytes
	}

	ids := make([]string, 0, len(in.Files))
	for id := range in.Files {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	var out []Finding

	for _, id := range ids {
		for _, file := range in.Files.Attachments(id) {
			if file.Size <= int64(limit) {
				continue
			}

			out = append(out, Finding{
				Severity: SeverityFail, ID: id, Path: file.Path,
				Message: fmt.Sprintf(
					"%s is %d bytes, and %s is %d: an attachment is in every clone "+
						"of this repository for good",
					file.Name, file.Size, config.KeyAttachmentMaxBytes, limit),
			})
		}
	}

	return out
}
