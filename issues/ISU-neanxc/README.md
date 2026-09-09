---
schema: 1
id: ISU-neanxc
title: M4-S7 · `isu triage`
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-6mkqq5
blocked_by: ISU-jjtray
acceptance: a report can be triaged without opening a pull request, if and only if the repository has said that is allowed.
---
**Done** #9, 2026-08-31. **The first version read the issue at trunk, and so failed on precisely the
issues triage exists for** — `awaiting triage` is *defined* as a folder trunk has never seen. It
now reads the issue from wherever it is and writes it where it is going, which also makes
`--push` accept a report onto trunk in one commit. The branch it writes is `triage/<ID>`, chosen
here rather than in this document: it is deliberately outside the `isu/` namespace, because a
triage edit does not flip the state and so is not a claim, and the board says nothing about it
until it merges.
**Branch** `isu/M4-S7-triage`
**Why** Everything the plan can express about an issue other than its state — who owns it, what
blocks it, which epic it belongs to, how urgent it is — had no command. Six reports filed on a
Friday should not wait for Monday's review queue to become answerable.
**Build** `isu triage <ID>` setting `owner`, `parent`, `blocked_by` and `priority`. Defaults to
branch-and-pull-request like everything else. `--push` commits straight to trunk, and is
refused unless `.isu.yml` sets `direct_triage: true` — the field is how a team says out loud
that triage is not a code review.
**Tests first** each field round-trips; `--push` without the config flag is refused with a
message naming the flag; `--push` with it produces exactly one trunk commit; `parent:` naming a
non-epic is refused at the command rather than left for CI; unknown priority is refused; the
body and unknown keys survive byte-for-byte.
**Done when** a report can be triaged without opening a pull request, if and only if the
repository has said that is allowed.
