---
schema: 1
id: ISU-mz0xns
title: M0-S4 · The git test harness
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-3ymsy4
blocked_by: ISU-tdgkj4
acceptance: a five-line test can produce a repo with two branches and a squash merge. ---
---
**Done** #4, 2026-08-25. Commits by a second author are not in the harness yet; they
arrive with M3-S3, which is where contention needs them. Claim refs were listed here too
and are no longer a thing the harness will ever need — a claim is a branch, and the
harness already builds branches.
**Branch** `isu/M0-S4-gittest`
**Why** Every meaningful test in this project builds a real repository. Getting this helper
right early is the difference between fast tests and a swamp.
**Build** `internal/gittest` with a fluent builder: `New(t)`, `.Issue(id, opts...)`,
`.Commit(msg)`, `.Branch(name)`, `.Checkout(name)`, `.Merge(branch)`,
`.SquashMerge(branch, subject)`, `.Revert(ref)`, `.WithRemote()`, `.DetachRemote()`,
`.File(path, content)`, `.Backdate(days)`. Repos go in `t.TempDir()` and clean themselves up.
**Tests first** the harness tests itself: build a repo, assert the resulting `git log`,
`ls-tree` and branch topology are exactly as scripted.
**Done when** a five-line test can produce a repo with two branches and a squash merge.

---
