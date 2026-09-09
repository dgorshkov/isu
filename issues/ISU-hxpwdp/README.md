---
schema: 1
id: ISU-hxpwdp
title: M5-S2 · Structural checks
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ams73p
blocked_by: ISU-1ntxd5
acceptance: every rule in the data model that can be checked without a diff is checked.
---
**Done** #11, 2026-09-01. Six rules, and half of them are the other half of a sentence M1-S2
could only start: `Validate` takes one issue and nothing else, so `parent:` is checked for shape
there and for what it names here. **Every copy is checked and not only the one derivation
renders** — an issue that is fine at trunk and broken on the branch proposing it is a broken
issue, and the finding names the branch so nobody goes looking at trunk for it. **The
duplicate-id rule is about ids trunk has never seen**, and has to be: an id on trunk and on a
branch is one issue somebody edited, which is the model working. Two branches carrying an id
trunk has never seen, with different titles or creation dates, are two issues wearing one id.
Its first version got that wrong in a way only a two-branch fixture found, and M5-S5's
contention fixture is what found it: it meant to skip the ids trunk already carries and instead
appended the branch copies to the empty entry it had just made for them, so one issue claimed
on two branches read as a corruption. The rule now has two fixtures asserting it does *not*
fire — one issue edited on two branches, and a report triaged on a second branch — because a
rule that fires on the ordinary case is worse than no rule at all.
**Branch** `isu/M5-S2-structural-checks`
**Build** schema validity, id matches folder, duplicate ids, `parent` and `blocked_by` exist,
**`parent` names an issue of `type: epic`**, parent cycles, self-parent, dependency cycles,
**an epic declaring its own `state:`**, **an epic with no children**, and the attachment size
cap from `attachment_max_bytes`.
**Tests first** one repo fixture per violation, and one clean fixture asserting zero findings.
The duplicate-id fixture matters more than it used to: it is now the only thing standing
between two clones that generated the same token at the same moment and a corrupt tree.
**Done when** every rule in the data model that can be checked without a diff is checked.
