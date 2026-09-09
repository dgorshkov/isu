---
schema: 1
id: ISU-6mkqq5
title: M4 · CLI
type: epic
owner: dmitry
created: 2026-08-23
priority: p2
---
**Status** done — all eight stories landed in one pull request rather than eight. That was asked
for explicitly, as it was for M1, M2 and M3, and the working agreement now allows it outright. Eight commands in one
pull request sits at the top of what the working agreement's one-sitting test tolerates, and it is why that test is
written down: M5's seven checks do not depend on each other and are better off as two or three
pull requests than as one. The milestone boundary rule applies as ever — M5 does not start
without explicit approval.

**One correction, two defects and three limitations came out of this milestone**, and each is
recorded where the decision lives rather than only here: the claim design's compare-and-swap does
not hold between two claimants who share an identity, and the data model above is corrected; `isu
triage` read the issue at trunk and so failed on precisely the reports it exists for; a
repository whose trunk is called neither main nor master could not resolve anything; and M4-S8's
notes record what the board cannot see across a team, what `fetch_warn_hours` actually measures,
and what the trunk history walk costs per commit.

**M2-S1's grep is amended, narrowly.** Its rule is that internal/gitx is the only place that may
build a *git* command; its proxy was that nothing outside it names `exec.Command`. `isu comment`
opens `$EDITOR`, which is not a git command and cannot be made into one, so one file —
`internal/cli/editor.go`, named rather than matched by a pattern — is exempt from the grep and
asserted directly not to run git.
