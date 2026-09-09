---
schema: 1
id: ISU-g4702b
title: M6-S3 · Filter and navigation
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ahaqb3
blocked_by: ISU-97tpzx
acceptance: filtering a 5,000-issue fixture stays inside one frame budget.
---
**Done** #12, 2026-09-02. Every printable key goes into the needle while the filter line is
open, which makes the whole command map unreachable there — a `q` that quit half way through
typing "queue" would make the filter unusable — and the key hints change with it, because offering
`c claim` on a line that cannot claim is offering something that does not happen. The arrows still
move.

**The cursor remembers what somebody chose rather than where it happens to be.** A filter that
hides the selected issue moves the cursor; clearing it puts them back, because the thing they
picked is still what they picked. Only a deliberate move changes that, which is the difference
between the two halves of this story's own sentence.

**The frame budget was missed on the first draft and the fix was one line of design.** Lowercasing
five fields per issue on every keystroke is twenty-five thousand allocations a keypress on a
five-thousand-issue board: 5–8 ms against a 16 ms frame. Each issue's searchable text is
lowercased once at startup instead — it cannot change underneath the index, because nothing in
this package loads anything — and the same keystrokes now cost **2.6 ms**.
**Branch** `isu/M6-S3-tui-filter`
**Build** incremental filter across id, title, type, status and owner; vim and arrow keys;
selection preserved across filter changes where the selected issue still matches.
**Tests first** narrowing then clearing restores the previous selection; navigating across a
collapsed epic; filtering to zero results and back; a filter string containing regex
metacharacters is treated literally.
**Done when** filtering a 5,000-issue fixture stays inside one frame budget.
