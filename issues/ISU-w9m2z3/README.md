---
schema: 1
id: ISU-w9m2z3
title: M1-S2 · The Issue type and its schema
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-7076wc
blocked_by: ISU-4bkws4
acceptance: `Validate()` output is stable, sorted and human-readable, and the epic cases pass without the function ever seeing a second issue.
---
**Done** #5, 2026-08-26. Built in `internal/issue`, not `internal/model` — the latter is
M3-S1's derivation package, and the coverage gate's own test wrongly said it arrived here.
`priority` is decoded as written and defaulted at `EffectivePriority()` rather than in the
struct, or `Encode` would write `priority: p2` into a file whose author never typed it and
M1-S3's zero-diff property would be gone. `parent:` is checked for shape and not for what it
names: the frontmatter table read as though M1 enforced it, which would mean loading a second
issue to validate the first, so the data model now says out loud that it is M5-S2's.
**Branch** `isu/M1-S2-schema`
**Build** the `Issue` struct, the `Type` and `State` enums, and `Validate()` implementing the
required-field table from the data model. Errors accumulate — return all problems, not the first.
`Validate()` takes one issue and nothing else: no child index, no sibling lookup, no repo.
**Tests first** one case per row of the type table, plus: id not matching folder, missing
title, missing or malformed `created`, unknown type, unknown state, unknown priority, dropped
without `reason`, dropped without `resolution`, **dropped with a `resolution` outside the
enum**, bug without repro, **an epic declaring `state:`**, and **a non-epic omitting it**.
**Done when** `Validate()` output is stable, sorted and human-readable, and the epic cases pass
without the function ever seeing a second issue.
