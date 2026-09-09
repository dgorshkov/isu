---
schema: 1
id: ISU-g6psvy
title: M6-S4 · Detail pane
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ahaqb3
blocked_by: ISU-g4702b
acceptance: the detail pane answers "can I start this?" without leaving the TUI.
---
**Done** #12, 2026-09-02. "Can I start this?" is a question with four parts — what is it, has
anybody got it, what is it waiting on, and what does done look like — and the pane answers each:
the type's own required field, both claimants on a contended issue, every blocker with its own
status, and an epic's children with theirs. A child is told where in its epic it sits, because
being one of forty is a different proposition from being the last of three.

The body is rendered rather than printed, which is where glamour earns its place in the allowlist
and what `isu show`'s "v1.0.0 does not render markdown" was waiting for. A pane too narrow to wrap
into gets the body as it was written, which is the answer `isu show` gives anyway.

**What lives beside an issue is loaded, so it arrives through an action and is asked for once** —
the answer costs a git process and the cursor walks over the same issue every time somebody
scrolls past it. A folder that could not be read is kept as the failure it was: never failing
silently is M6-S5's rule and it applies to the read as much as to the writes.

`enter` gives the keys to the pane so it can be scrolled and `esc` gives them back. The wrap this
story needed also fixed one M6-S1 had shipped: folding rebuilt a line out of its words, which
collapsed the two spaces holding a key away from its value, so the first fold in a pane unaligned
every column in it.
**Branch** `isu/M6-S4-tui-detail`
**Build** body rendered with `glamour`, title, priority, claim line, acceptance/repro,
attachments, comment list, epic position, and `blocked_by` showing each blocker's own status.
**Tests first** golden frames for each type; an issue with twenty attachments scrolls; a
contended issue shows both claimants.
**Done when** the detail pane answers "can I start this?" without leaving the TUI.
