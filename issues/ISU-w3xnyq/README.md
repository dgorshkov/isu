---
schema: 1
id: ISU-w3xnyq
title: M3-S2 · Epic rollup
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-stb9h3
blocked_by: ISU-1kckxh
acceptance: a 5,000-issue fixture rolls up 100 epics in under 50 ms.
---
**Done** #8, 2026-08-27. The fold marks each epic with one of three states — not started,
part-way through, finished — and meeting a part-way one is the cycle: the parent chain came
back round to where it started, so the answer is a value rather than one more stack frame. An
epic that is its own parent is the one-node version, and it is the case that blew a stack
during prototyping. Every child is folded even after the first unfinished one, because stopping
early is the same answer for *this* epic and a different one for the board — an epic further
down that nothing else points at would keep whatever status the walk happened to leave it with.
One case the story does not name: an epic reported on a branch and not yet on trunk reads
`awaiting triage` rather than folding, because a folder trunk has never seen is a report
whatever type it declares. The gate is met with room: every fifth issue is an epic, so 5,000
issues is **1,000** epics against the story's 100, and deriving the whole board takes **5.2 ms**
against a 50 ms budget — 10.4 ms when the same measurement was taken over a board that had been
read out of a repository, where the issues arrive spread across memory rather than allocated in
one pass. **The fixture is built in memory rather than generated as a repository**,
and that is not a shortcut: this story measures a fold over issues that are already loaded, and
generating 5,000 issue folders to read them back is half a minute of git this test does not
time. It is also half a minute spent on a CI runner that is at that moment timing M2-S5's read
path in another process — which is exactly how it made that gate fail twice before anybody
noticed the two were fighting. See M2-S5.
**Branch** `isu/M3-S2-epics`
**Build** children indexed once per load, not scanned per parent. Fold child states into every
`type: epic`. Cycle-safe: a parent cycle must return a value, never recurse forever.
**Tests first** rollup with mixed children; all dropped; nested epics three deep; **an epic
that is its own parent**; a two-node parent cycle; an epic with no children; a `parent:` naming
an issue that is not an epic. The self-parent case blew a stack during prototyping — write it
before the implementation.
**Done when** a 5,000-issue fixture rolls up 100 epics in under 50 ms.
