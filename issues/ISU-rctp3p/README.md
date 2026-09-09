---
schema: 1
id: ISU-rctp3p
title: M2-S3 · Load from the working tree
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-51kzyr
blocked_by: ISU-8c45t5
acceptance: the agreement property holds on a 5,000-issue fixture.
---
**Done** #6, 2026-08-26. **"No git process at all" below is wrong by one, and the line after
it is why.** Honouring `.gitignore` is in the same sentence, and the two cannot both be
true: the rules live in the repository, in `$GIT_DIR/info/exclude` and in the user's global
excludes file, so answering by hand means reimplementing them and then disagreeing with git
about a corner of them. `git ls-files` answers once, in constant time, and one process is
still nothing beside the per-blob path. The walk itself needs none: issue folders are one
level under `issues/`, so it is one directory listing and one read per issue rather than a
tree walk. A README that is a symlink is refused with a reason rather than resolved — it
reads as its target on disk and as the target's *path* at a ref, which is the one way these
two loaders could disagree about a clean checkout. The root is the `Repo`'s rather than an
argument.
**Branch** `isu/M2-S3-load-worktree`
**Why** `isu ui` reads what is on disk, uncommitted edits included. That is a different path
from `LoadRef`, and a faster one — no git process at all. M6 depends on it, so it is built here
rather than discovered there.
**Build** `repo.LoadWorktree(root)` walking `issues/` and returning the same map type as
`LoadRef`. Honours `.gitignore`. An issue that parses but does not validate is reported, not
fatal — a half-written issue must not blind the whole board.
**Tests first** an uncommitted new issue and an uncommitted state flip both appear; a
malformed issue is reported rather than fatal; and a property test asserting `LoadWorktree`
and `LoadRef` agree exactly on a clean checkout of a generated repo.
**Done when** the agreement property holds on a 5,000-issue fixture.
