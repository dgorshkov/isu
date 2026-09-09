---
schema: 1
id: ISU-ram4wv
title: M2-S5 · The performance gate
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-51kzyr
blocked_by: ISU-vj4288
acceptance: both gates pass in CI on the slowest runner. ---
---
**Done** #6, 2026-08-26. Both gates pass with room: `LoadRef` **273 ms** against 1.5 s, and
the whole board **1.42 s** against 6 s, on the CI-class machine the branch was built on.
**The board needed a different algorithm from the one the story implies.** Listing every
ref's whole tree is a million tree entries and a million map entries over 200 branches, and
it took 11.4 s against a budget of 6. What the board wants to know about a branch is how it
differs from trunk, which is a handful of files, and git answers that in time proportional
to the difference because it compares trees by object id and skips the subtrees that match.
So trunk is listed once, every other ref is diffed against it, one `cat-file --batch` reads
the union of the blobs, and each ref's set is trunk's map cloned with its own changes over
the top. The process count is unchanged at refs + 3 — one `for-each-ref`, one `ls-tree`,
one `diff-tree` per other ref, one batch — so the assertion the story asks for still holds.
A consequence worth knowing before M4 writes anything: **an issue is shared between the
refs whose file is identical**, which is what keeps 200 branches from being a million issue
files in memory, and makes a `Board` a read model rather than something to write through.
The fixture generator builds trunk by writing files and staging them once, and its branches
with a single `git fast-import`; checking out a branch per issue was measured at 133 ms a
branch against 2 ms, which on 200 branches is half a minute of a test doing nothing anybody
asked about.

**#8 made this gate fail twice on macOS, and the cause was not what the first fix said it was.**
`LoadRef` missed the 1.5 s budget at 1.587 s under `make cover`, and then at **1.955 s under
`make test`** — the second in an uninstrumented binary, which rules instrumentation out.
Diagnosing the first failure as instrumentation cost was wrong, and the second run is what said
so. What both runs had in common: M3 added a package whose own gate generated a 5,000-issue
repository, and Go runs package tests concurrently, so a second 5,000-issue fixture was being
built on the runner while this one was being timed. **The budgets here are unchanged, and the
contention is gone** — M3-S2's fixture is built in memory, because what that story measures is
a pure fold and not a repository.

**The clock carries an allowance for instrumentation.** Instrumentation costs something real —
`-coverpkg=./... -covermode=count` takes this package from 1.42 s to 3.82 s on a development
machine — so a binary built for coverage is held to its own, looser number. Two is an allowance
rather than a model, but it is bracketed above by every slower algorithm the working agreement measured:
`ref:path` at 4.9 s and a `git show` per file at 13.7 s, both outside the 3 s this gives
`LoadRef` before instrumentation is added to them, and listing every ref's whole tree at 11.4 s
against the 12 s this gives the board. So it still separates this algorithm from the ones it
replaced, which is what the gate is for. A failure names which budget it was held to. The
process count — which this story says is the assertion that actually prevents the regression —
is asserted in every pass and was never affected by any of this.

**The lesson worth keeping: a wall-clock gate is a claim about the whole machine, not about the
code under it.** Any later story that adds a test heavy enough to run beside this one is
changing this gate's inputs whether it means to or not.

**#11 made it fail a third time, and #8's diagnosis above was incomplete.** `LoadRef` missed
the budget at **1.867 s under `make test`**, with `internal/repo/ref.go` untouched since M2 and
the process count passing — so, again, not the code this gate guards. Removing M3-S2's
competing fixture removed one contender, not the concurrency that made it matter, and
"the contention is gone" was too strong. Two things were wrong with the budget itself, and the
amendment below corrects both.

**1.5 s was never a budget for the slowest runner in CI, which is what this section says these
budgets are.** It was measured at 273 ms on a Linux-class machine and never checked against the
other half of the matrix. Across one CI run of one commit, where the only difference is the
runner, `internal/repo` took **22.9 s on `ubuntu-latest` and 69.2 s on `macos-latest`**, and
`internal/cli` 24.2 s against 69.9 s. **macOS is three times slower at this workload**, which
turns 273 ms into something near a second before anything else is running, and leaves a 1.5 s
budget with no margin at all. `slowRunnerFactor` is that measured factor, rounded down to two
and applied on darwin only — named for the platform it was measured on rather than for "not
linux", because M9-S1 adds a Windows build and a budget that loosened itself on a platform
nobody had measured would be a number with no evidence behind it. It stays bracketed above by
the algorithms this gate separates this one from: scaled by the same measured three, `ref:path`
costs about 15 s on that runner and a `git show` per file about 41 s, both far outside the 3 s
this now gives `LoadRef` there, and listing every ref's whole tree about 34 s against the 12 s
it gives the board.

**And the measurement was never taken alone, which is this story's own lesson arriving a third
time.** #8's fix removed one competing fixture; it did not remove the concurrency that made it
matter. `go test ./...` runs packages concurrently and `internal/cli` — seventy seconds of git
on macOS — runs beside this package for the whole of it. Measured here under controlled
oversubscription on three cores, `LoadRef` goes **242 ms, 374 ms, 494 ms, 764 ms** at nothing,
two, four and eight competing processes: roughly linear in the oversubscription. M6 has since
added `internal/ui` to that set and M7 through M9 will add more, so the number was going to keep
drifting. **So the clock is no longer asserted beside anything.** `make perf` runs this package
alone, in one process, and sets `ISU_PERF` to say the measurement has the machine; `make test`
and `make cover` still run these tests and still log what they measured, which `go test -v`
shows, but only that target holds the figure to a budget. `make perf` passes `-v` itself, so
the authoritative number is on the record of every CI run — this section asks for one line to
revisit if a run ever comes back close to the budget, and that is only possible if the run says
what it measured. The budgets themselves are otherwise the plan's, unchanged.

**The quiet gate's first macOS numbers say how close all of this was.** With the machine to
itself, `LoadRef` measures **707 ms** there and the whole board **3.66 s**, against 274 ms and
1.41 s on the machine the budgets were taken on — a ratio of 2.58 and 2.60, measured on the
operations themselves rather than inferred from package times. So the plan's 1.5 s and 6 s left
**2.12× of headroom for `LoadRef` and 1.64× for the board** on that runner with nothing else
running at all, before any of the contention above. **The board was the tighter of the two the
whole time.** `LoadRef` is simply the gate whose luck ran out first; the contention that put it
through 1.5 s would have put the board through 6 s too. That is why the allowance applies to
both budgets rather than to the one that went red, and it is the number to watch: every CI run
now prints it.

**What did not change, and is the reason this was only ever a red build and never a defect:**
the process counts. They are asserted in every pass, they are what this story says actually
prevents the regression, and no amount of contention can move them.
**Branch** `isu/M2-S5-perf-gate`
**Build** a generator producing an N-issue, M-ref fixture repo, `BenchmarkLoadRef` and
`BenchmarkBoard`.
**Tests first** `TestLoadRefUnder5000IssuesIsFast` builds 5,000 issues and **fails if
`LoadRef` exceeds 1.5 s**. Also assert that the number of `git` processes spawned is exactly
2, regardless of issue count — that is the assertion that actually prevents the regression,
because the slow paths differ by process count, not by algorithm.
Then the same discipline over refs, because `isu board` never loads one ref: build **200
branches over 5,000 issues** and assert a whole-board budget of **6 s** and a process count
that grows linearly in refs and not at all in issues. The 0.6 s in the read path was measured on a
single ref; it is not a number the product's main operation can be held to.
**Done when** both gates pass in CI on the slowest runner.

---
