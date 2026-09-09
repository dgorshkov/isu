---
schema: 1
id: ISU-1kckxh
title: M3-S1 · Status derivation
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-stb9h3
blocked_by: ISU-51kzyr
acceptance: the status function has 100% branch coverage. This package is the product; it gets a higher bar than the rest.
---
**Done** #8, 2026-08-27. **The loader grew rather than derivation reaching for git**, which is
§M3-S1's own rule taken at its word: `repo.Board` carries a `Changed` index naming the ids each
ref differs from trunk on. A board is a statement about differences and a ref's map is a
statement about contents; asking each ref for its whole map is five thousand issues two hundred
times over, and the diff that built the ref already knew which handful of files it was not.
`reopened` landed here rather than in M3-S4, because the table has six rows and this story owns
the table — the fold sits in `reopen.go`, which is where M3-S4's own tests point. Two cases the
story's list does not name and the model forces: a branch that *deleted* an issue's folder is
not evidence about anything, because deriving from an absence would let one branch take an
issue off everybody's board; and a non-epic whose `state:` is missing or misspelt matches no
row at all, so it reads `open` and `isu check` reports the file. **A trunk file that will not
decode at all is the same answer with an annotation**, added after review: the folder is
trunk's, so the issue is `OnTrunk` and reads `open`, and `Item.Broken` says why nothing more
can be said about it. Derivation first read only `Set.Issues`, which dropped the issue from
the board entirely — undoing, silently and for the one issue somebody most needs to hear
about, the loader's deliberate choice at §2 that a half-written issue must not blind the whole
board. Where a branch carries a readable copy it is rendered from, but never believed: the
issue used to read `awaiting triage`, calling a years-old issue a report trunk has never seen,
and a branch saying `resolved` must not finish an issue whose trunk state nobody can read.
**"100% branch coverage" is
measured as 100% of statements**, because that is what `go test -cover` counts and there is no
branch-coverage mode to turn on; the gate in `scripts/coverage.sh` enforces it, and every row
of the table has a test of its own on top — including both sides of each `&&` in the ladder,
which is the part a statement count would otherwise let through.
**Branch** `isu/M3-S1-status`
**Build** `internal/model`: given trunk, every branch and the history index from M2-S4, derive
the six statuses from the table in the data model. **Pure function over loaded inputs — no git calls
inside, and no exceptions to that rule later.** Everything a status needs is loaded by M2 and
passed in; if a future status needs something else, the loader grows, not this package.
**Tests first** one test per status, then the transitions between them. Include: issue on two
branches, issue on a branch identical to trunk, branch deleted after merge, a branch that
resolves an issue trunk has already resolved, and **a branch that exists without flipping the
state, which is not a claim and must read `open`**.
**Done when** the status function has 100% branch coverage. This package is the product; it
gets a higher bar than the rest.
