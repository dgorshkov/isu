---
schema: 1
id: ISU-7w8xhg
title: M3-S4 · Reopen detection
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-stb9h3
blocked_by: ISU-jv2gf7
acceptance: reopen is correct with zero branches left in the repo.
---
**Done** #8, 2026-08-27. **The fold itself landed with M3-S1**, because the status table has a
`reopened` row and that story owns the table; this story is the case matrix over it and the
purity tests, and it found two things the story's list does not expect.

**Merge-then-revert does not read the same with the branch deleted and without it.** The list
below asks for the three cases "and the same three with the branch deleted after merge", which
reads as though the answers match. They do not, and the table is why: a branch left standing
through a revert still says `resolved` where trunk now says `open`, which is a claim by the
only definition of one there is, and `in progress` beats `reopened`. So it reads `reopened`
with the branch gone and `in progress` with it there. Nothing is lost — the annotation survives
either way — and the board is saying that somebody's branch disagrees with trunk, which is
exactly the situation.

**A deletion in the middle of an issue's history does not break the reopen chain — asked here,
answered in #8.** An issue whose folder was deleted at trunk and later written again holds a
removal between its states. The rule as the table states it makes that a reopen; M2-S4's
reading of a rename would have made it a different issue's history. The rule as written stands:
an id is permanent from creation, so the same id is the same issue, and trunk did resolve it
once. The data model now says so where the rule lives, rather than only here.

Two things the list does name and are worth keeping visible: `dropped` at an earlier commit is
deliberately *not* a reopen, because the table names `resolved` and undropping is a triage
decision rather than a fix that did not hold; and the purity rule got three tests rather than
one grep — what this package may import, that it may not name a `*repo.Repo`, and that a whole
derivation moves the repository's process counter by zero.
**Branch** `isu/M3-S4-reopen`
**Build** a **pure fold over the M2-S4 history index**: if any earlier entry was
`state: resolved` and the latest is `open`, the status is `reopened`. No git in this package —
that is the whole reason M2-S4 exists.
**Tests first** merge then revert reads as reopened; merge, revert, re-fix reads as done;
never-resolved reads as open; **and the same three with the branch deleted after merge**. Plus
a test asserting this package spawns no processes, in the style of the M2-S1 grep test.
**Done when** reopen is correct with zero branches left in the repo.
