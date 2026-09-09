---
schema: 1
id: ISU-b2bwv8
title: M2-S1 · `internal/gitx`
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-51kzyr
blocked_by: ISU-7076wc
acceptance: `grep -r "exec.Command" internal/ | grep -v gitx` returns nothing. Add that grep as a test.
---
**Done** #6, 2026-08-26. `gittest` now runs its own git through this package. The done
condition below is a grep that must return nothing, and the harness was the one thing that
would have kept it returning a line; an exemption list would have made the rule advisory on
the day it was written, so the harness hands `gitx` an environment function instead — the
only caller of `WithEnv`, and the only legitimate reason for git to see something other
than what the user configured. `ErrUnknownRevision` is a typed kind, because M2-S2 has to
tell an unborn HEAD from a typo and the two are the same exit status and nearly the same
sentence. Three wrappers joined the list below: `Feed`, for the commands that read a stream
rather than arguments, and `DiffTree`, which M2-S5 turned out to need — plus `Show`, which
was already named. A "clean environment" means the repository-selecting `GIT_*` variables
never reach git, and everything else does: the user's config, credential helpers and hooks
are the whole reason the working agreement shells out.

**M4-S6 amended the grep, narrowly, and it is worth reading as a lesson about proxies.** The
rule is that this package is the only place that may build a *git* command; the grep is that
nothing outside it names `exec.Command`. The two were the same thing until `isu comment` had to
open `$EDITOR`, which is not a git command and cannot be made into one. So the test names one
file — `internal/cli/editor.go`, named rather than matched by a pattern, so that a second
process-starting file is a deliberate edit to the test — skips it, and asserts of it the thing
the rule is actually about: that it does not run git. Everything else in `internal/` is still
held to the grep. The alternative was dropping `$EDITOR`, and a tracker whose comments can only
be written with `-m` is a tracker people stop commenting on.
**Branch** `isu/M2-S1-gitx`
**Build** the only place that executes `git`. Typed wrappers for `ls-tree`, `cat-file --batch`,
`log`, `rev-parse`, `for-each-ref`, `diff --name-only`, `push`, `show`. Context-aware, with
timeouts. Errors carry stderr. Detects git absence at startup with a clear message.
**Tests first** each wrapper against a `gittest` repo; an error case per wrapper; a test that
asserts the binary is invoked with `--no-pager` and a clean environment.
**Done when** `grep -r "exec.Command" internal/ | grep -v gitx` returns nothing. Add that
grep as a test.
