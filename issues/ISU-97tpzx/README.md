---
schema: 1
id: ISU-97tpzx
title: M6-S2 · List and grouping
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ahaqb3
blocked_by: ISU-955230
acceptance: the grouping matches `isu board` exactly, asserted by a shared fixture.
---
**Done** #12, 2026-09-02. The grouping is one function used twice rather than two orderings
that agree: `view.itemGroups` buckets by derived status in the precedence order of the data model's table,
`isu board` renders it as JSON and `isu ui` hands the same slice to the interface. That is what
"matches exactly" can mean and keep meaning, and a shared fixture in `internal/cli` asserts it
against a real repository.

**Within a group the order is the board's, with one thing done to it**: a child whose epic is in
the same group is drawn immediately after that epic, one level in. Which group an issue is in is
untouched, so a child whose epic has finished stands at the top of its own group rather than under
an epic three groups away — the alternative, nesting across groups, would have meant a list whose
statuses no longer add up to the counts above them. A folded epic says how many rows it is holding
back beside its title rather than beside its id, because a marker glued to an id is a marker
somebody copies with it.

Every issue is emitted once. A parent cycle is two epics that are each other's ancestors, which is
`isu check`'s to report and this arrangement's to survive: the walk marks what it drew and a sweep
at the end draws whatever it could not reach.
**Branch** `isu/M6-S2-tui-list`
**Build** the issue list with epics as parents and their children indented beneath, status
colouring, and the counts in the header staying consistent with the list beneath them.
**Tests first** golden frames for a list with nested epics; an epic with forty children; a
list of zero issues rendering the empty state rather than a blank pane.
**Done when** the grouping matches `isu board` exactly, asserted by a shared fixture.
