---
schema: 1
id: ISU-q4dy32
title: M5-S6 · Hooks and CI templates
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ams73p
blocked_by: ISU-9m7jw9
acceptance: the generated pipeline runs `isu check` and fails the build correctly.
---
**Done** #11, 2026-09-01. **The hook this story specifies cannot be written as specified, and
the correction is `isu check --worktree`.** Checks read refs, by M5-S1's rule; a pre-commit hook
that reads refs is answering about the commit *before* the one being made. That is not a slower
answer, it is a deadlock: commit a broken issue file — the hook sees the previous commit and
allows it — then try to commit the fix, and the hook sees the broken file that is no longer
there and refuses. So `isu check` grew `--worktree`, which reads the issues on disk through
M2-S3's loader and a new `repo.LoadWorktreeFiles` beside it, and the hook runs that. It implies
`--scope tree`, because the working tree is not a set of commits and there is nothing there for
the branch rules to be about.

Beside that: the hook goes wherever git looks for hooks rather than into `.git/hooks`, since
`.git` is a directory in a clone, a file in a submodule and a file in a linked worktree, and
`core.hooksPath` moves the lot — a hook in the wrong one of those is a hook that silently never
runs. It exits 0 with a word on stderr when isu is not on `PATH`, because a hook that stands
between somebody and their commit for that is a hook the whole team deletes on its first day.
Idempotence is three rules over every file: write what is missing, leave what is already right,
refuse to overwrite what is neither without `--force` — which is also why `--prefix` is now
optional in a repository that already has a configuration, so that adopting isu after you
already have a pipeline adds the parts you are missing and keeps the parts you have. The
generated workflow fetches the base branch by name and passes it as `--ref`, because a pull
request build checks out a merge commit and no branch; without that isu compares the repository
against itself and every branch rule passes silently.
**Branch** `isu/M5-S6-init`
**Build** `isu init` writing a pre-commit hook and `.github/workflows/isu.yml`. Flags
`--hooks`, `--actions`. Idempotent: running twice changes nothing. Never overwrites an existing
file without `--force`.
**Tests first** init into a clean repo produces working files; init twice produces a
zero-length diff; init over an existing workflow without `--force` refuses.
**Done when** the generated pipeline runs `isu check` and fails the build correctly.
