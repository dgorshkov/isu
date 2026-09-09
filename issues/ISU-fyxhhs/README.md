---
schema: 1
id: ISU-fyxhhs
title: M4-S8 · First contact with a real repository
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-6mkqq5
blocked_by: ISU-neanxc
acceptance: all three load, and every surprise is either fixed or written down as a known limitation. ---
---
**Done** #9, 2026-08-31. Three full clones — openssl/openssl for its 11 submodules and 26 years,
facebook/react for its 967 remote branches and its squash-merge habit, golang/go for the biggest
tree. All three load; `docs/field-notes.md` has the timings and the ref counts. One defect found
and fixed with a regression test, and three limitations written down, of which one is serious
enough to belong here: **the board reads `refs/heads/` and so cannot see anybody else's claims.**
A claim reaches other people as `refs/remotes/origin/isu/<ID>`, which the board does not read, so
contention across a team — the thing claiming exists to prevent — is invisible. The freshness
line is the evidence that this is not what was intended, since fetch age cannot affect contention
unless remote refs feed it. Reading `refs/remotes/` costs 3.5 ms a ref, measured. It is not fixed
here because the ref pattern is M2's and the self-contention it would cause — a claimant's own
board reading their local branch and its remote-tracking twin as two claims — is M3's. **It wants
a story of its own, and it should get one before M5-S5 reports contention to anybody.**
**Branch** `isu/M4-S8-field-notes`
**Why** Everything so far has run against fixtures written by the same person who wrote the
assumptions. This is the first story where the world gets a vote, and it is deliberately
before the TUI and the importers are built on top of those assumptions.
**Build** run the CLI's read paths against three real repositories of different shapes: a
large monorepo, one with submodules, and one with a decade of history and heavy squash-merge
use. Read only — nothing is written and nothing is imported. Record what happened in
`docs/field-notes.md`, including the timings and the ref counts.
**Tests first** every defect found becomes a regression test with a fixture reproducing it,
written before the fix.
**Done when** all three load, and every surprise is either fixed or written down as a known
limitation.

---
