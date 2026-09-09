---
schema: 1
id: ISU-sbhnj0
title: M4-S2 · `isu board` and `isu show`
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-6mkqq5
blocked_by: ISU-hh9ry2
acceptance: `isu board` on the isu repo itself renders this plan's milestones.
---
**Done** #9, 2026-08-31. Both print the freshness line and warn past `fetch_warn_hours`. **The
`Issue` payload grew the five fields a type requires** — `repro`, `acceptance`, `question`,
`reason`, `resolution` — on every issue rather than only on `isu show`, because M4-S3 wants
`isu ready --json | head -1` to be the whole briefing and what somebody picking work up reads
first is the acceptance criteria or the repro. The body stays one `isu show` away. **The body is
printed verbatim rather than rendered**: glamour is in the working agreement's allowlist for the TUI, where a
renderer earns its place, and it is not needed to print a paragraph. `isu board` renders this
repository, which is the story's own done-when, and a test runs it against this working copy.
**Branch** `isu/M4-S2-board-show`
**Build** the derived board grouped by status, and single-issue detail including attachments,
comments, claim and epic position. Both print a **freshness line** — how old the newest remote
ref is — and warn once it exceeds `fetch_warn_hours`. Two engineers looking at the same repo with
different fetch ages see different contention, and the output should say so rather than let
them argue about it.
**Tests first** golden output for a repo in every status; `--json` round-trips through
`encoding/json` into the documented struct; a repo fetched 3 days ago renders the warning and a
freshly fetched one does not.
**Done when** `isu board` on the isu repo itself renders this plan's milestones.
