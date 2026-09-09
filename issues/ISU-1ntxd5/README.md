---
schema: 1
id: ISU-1ntxd5
title: M5-S1 · The check engine
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ams73p
blocked_by: ISU-6mkqq5
acceptance: adding a check is one file and one registry line.
---
**Done** #11, 2026-09-01. The engine is a registry, an order and a summary. Findings are
stamped with the name of the check that produced them by the registry rather than by each
check, so a check cannot disagree with the name it was registered under; checks run in name
order so that where a registry line sits in a file cannot change a pipeline's output; and the
report sorts failures first and then by check, issue, path and message, because a pipeline's
output is diffed by whoever is working out what their commit changed. **Two loaders arrive with
it**, because "checks receive loaded refs and the diff against trunk" is this story's own
sentence: `repo.LoadBranch` reads what a ref proposes over trunk — measured from the merge base,
so a trunk that has moved on does not read as the branch reverting work it never touched — and
`repo.LoadFiles` reads what lives beside each issue's README with its size, which is the only
source the attachment cap has. `repo.BoardSpec` also grew `Refs`, for the one case the patterns
cannot reach: a pipeline checks out a merge commit and no branch, and a check suite that could
not see the issues the pull request adds would pass every pull request that added a broken one.
**Branch** `isu/M5-S1-check-engine`
**Build** `internal/check`: a `Check` interface, a registry, severity levels (`fail`, `warn`),
and a reporter with human and JSON output. Checks receive loaded refs and the diff against
trunk — never raw git.
**Tests first** engine ordering is deterministic; a failing check exits 1, warnings exit 0;
`--json` output validates.
**Done when** adding a check is one file and one registry line.
