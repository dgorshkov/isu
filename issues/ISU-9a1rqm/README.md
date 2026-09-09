---
schema: 1
id: ISU-9a1rqm
title: M9-S3 · Release candidate hardening
type: story
state: open
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-thyf24
blocked_by: ISU-8za9zb
acceptance: all five scenarios pass and coverage is at or above the gate.
---
**Branch** `isu/M9-S3-hardening`
**Build** no new features. Fix what the following surface: run `isu` against three real
repositories of different shapes; run the full lifecycle under a squash-merge-only repo; run
with the remote detached for the whole session; run with 20,000 issues; and run with **500 live
branches**, because ref count is the dimension the board's cost actually scales in and the one
least exercised by everything above it.
**Tests first** convert every defect found into a regression test before fixing it.
**Done when** all five scenarios pass and coverage is at or above the gate.
