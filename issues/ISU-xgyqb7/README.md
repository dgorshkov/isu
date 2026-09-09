---
schema: 1
id: ISU-xgyqb7
title: M5-S3 · Evidence checks
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ams73p
blocked_by: ISU-hxpwdp
acceptance: an agent cannot mark work done without doing it.
---
**Done** #11, 2026-09-01. **One carve-out, and it is a decision this document did not make.**
Resolving means the branch found the issue open here and left it terminal. An issue the branch
*created* in a terminal state is not a resolution: it was never open in this repository, which
is exactly what an import is — M7-S2 writes thousands of issues Jira closed years ago, on a
branch that changes nothing outside `issues/` because there is nothing else to change. A rule
that demanded code for those would catch nobody and would make the importer unimplementable.
The `reason` and `resolution` a drop requires stay the schema's to report: `state: dropped`
without them does not satisfy `Validate`, and two rules reporting one line would give a reader
two things to fix that are one thing. A drop that also changes code warns and exits 0, as
specified.
**Branch** `isu/M5-S3-evidence-checks`
**Build** the type table's resolution rules: resolving requires a change outside `issues/`;
a spike requires an artifact in its own folder; a drop requires `reason` and `resolution`.
A drop that also changes code is a **warn, not a fail** — closing a duplicate in the same pull
request as the fix is a normal thing to do, and refusing it just teaches people to split the
work into two reviews.
Only applies to issues the branch actually changed.
**Tests first** resolved-with-no-code fails; resolved-with-code passes; **a branch carrying
nothing but a claim fails, and that is the check doing its job** — claiming writes
`state: resolved` and touches nothing else, so this check is exactly what separates a claim
from a resolution and the board's `in progress` from `done`; spike with only `README.md` fails;
spike with `decision.md` passes; drop without `resolution` fails; drop with code changes warns
and exits 0.
**Done when** an agent cannot mark work done without doing it.
