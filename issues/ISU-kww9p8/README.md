---
schema: 1
id: ISU-kww9p8
title: M4-S5 · `isu resolve` and `isu drop`
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-6mkqq5
blocked_by: ISU-44h3cz
acceptance: the only way to reach `done` is a merged pull request.
---
**Done** #9, 2026-08-31. **`resolve` allows a commit that changes no file**, which needed a second
gitx spelling: this story asks for exactly that case — resolve on a freshly claimed issue — and
the trailer is that commit's whole payload. `Commit` still refuses an empty index everywhere
else, because a command that meant to change a file and did not is a bug an empty commit would
hide. **`drop` writes the `Isu-Resolves:` trailer too**, which this story names only for
`resolve`: a dropped issue reaches a terminal state at trunk exactly as a resolved one does, and
the trailer is the only tier that survives every squash setting, so leaving it off would make
M7-S3's recovery silently partial for half the terminal commits in a repository.
**Branch** `isu/M4-S5-resolve-drop`
**Build** flip state on the current branch. `resolve` writes `state: resolved` and an
`Isu-Resolves: <ID>` trailer on its commit. `drop` requires `--reason` and `--resolution`.
Neither command merges anything; both leave a branch for a pull request.
**Tests first** resolve on a spike without an artifact warns; resolve writes the trailer and it
survives a squash merge; drop without a reason is refused; drop without a resolution is
refused; both refuse to run directly on trunk; **resolve on a freshly claimed issue leaves the
file byte-identical and writes only the trailer** — the claim already wrote `resolved`, so what
`resolve` adds is the `Isu-Resolves:` link and the code beside it, and a resolve that changes
nothing at all outside `issues/` is M5-S3's to reject.
**Done when** the only way to reach `done` is a merged pull request.
