---
schema: 1
id: ISU-9m7jw9
title: M5-S5 · Contention and staleness reporting
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ams73p
blocked_by: ISU-t5tjsg
acceptance: both surface at pull-request time rather than at merge.
---
**Done** #11, 2026-09-01. Both are warnings and neither fails a pull request: one refused for
contention is one refused for somebody else's branch. **Every finding carries a clause naming
what it was read from**, which is this story's own requirement and also the honest answer to
M4-S8's open limitation — a repository with no remote is told these are local branches only, a
stale ref set is told how old it is and to run `--fetch`, and a repository with a remote is told
that a claim somebody else pushed and never merged is not in the answer at all. That last clause
is a stopgap for the story M4-S8 asked for and is not a substitute for it.
**Branch** `isu/M5-S5-contention`
**Build** warn when another branch claims the same issue, naming the branch and holder; warn on
claims older than `stale_days`. Both warnings are statements about refs, so both are
only as true as the last fetch: run `--fetch` in CI, and include the fetch age in the warning
so a local run that disagrees with CI is self-explaining.
**Tests first** two branches claiming the same issue produces one warning naming the other
branch; stale claim produces a warning with the age in days; a stale local ref set produces a
warning that says so rather than reporting confident nonsense.
**Done when** both surface at pull-request time rather than at merge.
