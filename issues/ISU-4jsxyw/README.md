---
schema: 1
id: ISU-4jsxyw
title: M7-S3 · Resolving-commit recovery
type: story
state: open
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-2wyps1
blocked_by: ISU-traxr8
acceptance: the scanner runs against a real repository and reports its coverage.
---
**Branch** `isu/M7-S3-evidence-scan`
**Build** one pass over history recovering issue-key → commit links at three tiers: key in a
commit message, key in a merge commit's branch name, key in a squash subject. (Issues resolved
after the switch to isu carry an `Isu-Resolves:` trailer and need none of this — these tiers
exist for the years of history that predate it.) Record which
tier produced each link; unlinked issues import with their resolution date only.
**A GitHub key is `#1234`, and it is ambiguous in a way `PROJ-1234` never was.** Issues and
pull requests are numbered from one sequence, so `Merge pull request #456 from …` names a pull
request, a squash subject ending `(#456)` almost always does too, and `#1234` turns up in prose
about nothing at all. Every tier match is therefore checked against the set of numbers actually
being imported and discarded when it is not one of them — which means this scan cannot run
before the issue list is in hand, and the story is ordered accordingly.
**Tests first** a fixture repo deliberately mixing all three conventions plus a long tail of
commits with no key; assert per-tier counts exactly; assert an issue matched at two tiers
records the stronger one; **assert a `(#456)` squash subject naming a pull request produces no
link**, and that the same subject does produce one when 456 is an imported issue.
**Done when** the scanner runs against a real repository and reports its coverage.
