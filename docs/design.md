# The design record

This is why isu is built the way it is: the decisions that were argued about, the measurements
that settled them, and the ones that were reversed after a test proved them wrong.

The [data model](data-model.html) and the [status table](statuses.html) say *what* the rules are.
This page says why, and it keeps the workings — including the drafts that turned out to be
wrong, because a design record that only lists the winning answer is a record you cannot argue
with.

## Where the plan went

isu was built from a single document, `PLAN.md`, that carried three different things at once: a
working agreement, a design record, and fifty-one stories in build order. Those have been split
to where each belongs, and `PLAN.md` is gone:

- **The stories** are issue folders under `issues/`, one per story, with the milestones as
  epics above them. Every story's brief, its `**Done**` record and the corrections it produced
  are in the issue's own body, which is where `isu show` reads them. That is the whole point of
  this tracker, and a build plan kept in a file beside it was the one thing this repository was
  not dogfooding.
- **The working agreement** — TDD, the commit format, one story per pull request, the milestone
  boundaries, the coverage floors and the definition of done — is in `CLAUDE.md`, which is read
  at the start of every session.
- **The design record** is this page.

Where a milestone note or a story body written before the split says *the working agreement*, it
means `CLAUDE.md`; where it says *the data model*, it means [that page](data-model.html) and
this one.

## The stack

| concern | choice |
|---|---|
| Language | Go 1.24+ |
| CLI | `spf13/cobra` |
| TUI | `charmbracelet/bubbletea`, `bubbles`, `lipgloss`, `glamour` |
| TUI tests | `charmbracelet/x/exp/teatest` |
| YAML | `goccy/go-yaml` — for `.isu.yml` and `source.yml`, not for frontmatter |
| Assertions | `stretchr/testify/require` |
| Release | `goreleaser` |

That is the entire dependency allowlist. Adding anything else requires asking first.

**The minimum was 1.23 until M1, and 1.24.0 until M6.** `golangci-lint` v2.5.0 needs 1.24 or
newer to build, so with a 1.23 directive `go install` switched toolchains — to whatever Go had
released most recently, resolved fresh on every CI run — and the lint gate went red the day one
of those releases arrived incomplete. A gate that fails on a schedule nobody controls is a
pipeline people learn to ignore, so the directive moved to 1.24 rather than the symptom being
pinned around. It moved again to **1.24.2** in M6, because three of the modules bubbletea and
lipgloss bring declare that and a module's directive must be at least its dependencies'. Still
no `toolchain` line: the directive is a minimum, and pinning a toolchain is the thing that went
wrong in the first place.

Frontmatter is parsed by hand because it is a flat key/value block, and the byte-for-byte
round-trip guarantee is easier to hold without a YAML serialiser reformatting it. `.isu.yml` and
`source.yml` are real YAML and get a real parser.

Markdown is rendered for the terminal only, by `glamour`. **v1.0.0 ships no HTML**, so there is
no markdown-to-HTML pipeline and no sanitiser in the allowlist — see
[what isu does not do](not-doing.html).

## Git access

**Shell out to the `git` binary. Do not use go-git.** Reasons, in order: the user's git config,
hooks, credential helpers and LFS all apply for free; plumbing commands give exact control over
the read path; and the fast load path below is only available through plumbing. `git` is a hard
runtime requirement and that is fine for a developer tool.

All git invocation goes through `internal/gitx`. Nothing outside that package may construct a
`git` command — **the test harness included**, which is where M2-S1 found the one exception that
would otherwise have been written into the rule on its first day.

## The read path

Loading every issue from a ref **must** be done as:

1. `git ls-tree -r <ref>` — take the object id and path of every `issues/*/README.md`
2. one `git cat-file --batch` process, fed **object ids, not `ref:path`**
3. parse blobs out of the single output stream

This is not an optimisation to do later. Measured on 5,000 issues:

| approach | time |
|---|---|
| `git show` per file | 13.7 s |
| `cat-file --batch` on `ref:path` | 4.9 s |
| `cat-file --batch` on object ids | **0.6 s** |

`ref:path` costs a tree walk per lookup. Object ids skip it. `internal/repo/perf_test.go`
enforces this with a benchmark that fails the build if it regresses, and it asserts process
*counts* and not only times.

**The board is a different question and gets a different answer.** The figures above are one
ref; `isu board` reads trunk and every branch, and listing each ref's whole tree is a million
entries over 200 branches — measured at 11.4 s, against a 6 s budget. So trunk is listed once,
each other ref is *diffed* against it, and one `cat-file --batch` reads the union of the blobs
they name. Git compares trees by object id and skips the subtrees that match, so a branch costs
its own difference and not the repository. Measured: **1.42 s** for 5,000 issues over 200
branches.

## Claims

**This section was rewritten after the design it described was tested and found to be paying for
something it did not need.** The previous version claimed through a parentless commit at
`refs/claims/<ID>`; what follows is the same guarantee, obtained from a branch, and the
paragraphs below say why each of the old version's supporting arguments does not hold.

`isu claim AR-7f3akq` does, in this order:

1. create branch `isu/AR-7f3akq` from trunk;
2. write `state: resolved` into `issues/AR-7f3akq/README.md` and commit it, subject
   `claim AR-7f3akq`;
3. `git push origin isu/AR-7f3akq`. **Do this before any work**, so the loser wastes nothing.

**The push is the compare-and-swap, and the state flip is not enough to make it one — the claim
commit carries a nonce as well. Corrected in M4-S4, which measured the hole.** The paragraph
below argued that two claimants produce two different commits because their author and timestamp
differ. Two claimants under one identity, in the same second, produce the *same* commit: same
tree, same parent, same author, same second, same message. Git answers the second push
`Everything up-to-date`, exit 0, and both of them believe they won. A hundred clones racing for
one issue produced **thirty winners**. Two agents sharing a bot identity is not an exotic case
for a tracker built for agents, and neither is one person in two clones.

`--force-with-lease=refs/heads/isu/<ID>:` — the third mechanism named at the end of this
section — does not close it either: git short-circuits on "up to date" before the lease is
evaluated. Measured, both of them.

What closes it is making this section's own sentence true. Atomicity comes from committing
something nobody else can have committed, so `isu claim` writes an **`Isu-Claim:` trailer
holding a fresh random value** on the claim commit. The second push is then rejected as a
non-fast-forward, which is exactly the mechanism described below; it needed the premise to hold.
With the nonce, a hundred racing clones produce one winner.

The measurement below stands, and is what the design looks like when the two claimants happen to
differ — which is most of the time, and was never the problem:

```
alice's claim commit: 677b54d  (author: alice)
bob's claim commit:   f7478e5  (author: bob)

alice:  * [new branch]      isu/AR-7f3akq -> isu/AR-7f3akq
bob:    ! [rejected]        isu/AR-7f3akq -> isu/AR-7f3akq (fetch first)
```

**Pushing a bare branch at the trunk tip would not work**, and that is the trap the earlier draft
was written to avoid: git accepts it as a no-op fast-forward and tells both claimants they
succeeded, measured as `[new branch]` for the first and `Everything up-to-date` for the second,
both exit 0. Atomicity comes from committing something nobody else can have committed, not from
the ref namespace — a claim ref was one way to get that, and a commit that flips the state is
another. (`--force-with-lease=refs/heads/isu/<ID>:` with an empty expected value is a third,
rejecting the second push with `stale info`. It is not needed here and is not used.)

**The claim writes `resolved` before the work is done, and that is deliberate.** A branch is a
proposal, not a fact: `state: resolved` on `isu/<ID>` says *this branch intends to resolve this
issue*, and it becomes a fact about the repository when the branch merges. Trunk is where state
is true. Until then the only thing that reads it is a board that renders it as `in progress`.

**Claiming does not get you past the evidence check.** Resolving a bug, a story or a chore
requires a change outside `issues/`, and resolving a spike requires an artifact in the issue's
own folder. A claim changes one line of one README and nothing else, so a branch that claimed an
issue and did no work fails that check exactly as it should — the check reads the diff, not the
field. The earlier draft's objection to writing `resolved` early, that every claim would then
look like a resolution, is not true for this reason.

**Claimant and claim time are the first commit on the claiming branch.**
`git log <trunk>..isu/<ID> --reverse` is the commit that flipped the state — the **first** record
of that walk; its author is the claimant and its author date is the claim time. That is one git
process per claiming branch — linear in refs, which is what the board already costs — where a
dedicated claim ref would have made it free. It is the one thing this design pays more for. The
branch *tip* is free from `for-each-ref` and is the **wrong** answer: it moves every time the
claimant pushes more work, so a claim would never age and `stale_days` would never fire.

**This paragraph said `--reverse --max-count=1` until M3-S3, and that pair returns the tip.** Git
applies the limit during the walk, which starts at the tip, and reverses what survived it; one
commit reversed is that same commit. Measured on a branch of three commits over trunk:

```
git log --reverse --max-count=1 main..topic  ->  third on branch
git log --reverse             main..topic  ->  first on branch, second on branch, third on branch
```

So the spelling above walks the range and takes the first record, which costs a claiming
branch's own commits rather than a constant. That is affordable — a claiming branch is a few
commits, not a repository — and it is correct, which the pair is not at any price.
`TestLoadFirstCommitsTakesTheFirstCommitAndNotTheTip` asserts the commit id rather than only the
date, so a lookup that is accidentally right on a one-commit branch cannot pass it.

`isu unclaim` flips the state back to `open` on the claiming branch and pushes. **It does not
delete the branch** — a one-word command must not throw away work. What it releases is the
claim; what it leaves is an ordinary branch the board says nothing about. Claims are advisory
for humans and binding for agents.

**A claim is released when the work lands, and there is no sweep.** Terminal trunk state wins the
[precedence table](statuses.html), so a merged issue reads `done` whatever its branch still says,
and the branch is the forge's to delete on merge. Nothing outside `refs/heads/*` accumulates, so
nothing has to remember to tidy it — where the earlier draft needed a CI job to delete
`refs/claims/<ID>`, without which contention reporting would flag a stale claim on every
finished issue for the life of the repository.

**Nothing here pushes outside `refs/heads/*`.** The remotes that refuse other namespaces are not
a special case, there is no `--no-claim` degraded mode to build, and a rejected push is
unambiguous: on a branch it is a lost race and never a policy refusal. The earlier draft paid for
all three — it needed a mode nobody would exercise, and it needed `isu claim` to tell two
identical-looking git failures apart and explain which one had happened.

## Squash-merge safety

Post-merge questions are answered from **file content at trunk commits**, never from commit
metadata. Squash collapses authorship; it does not touch the file. Every derivation that reads
history must read blob content, not `%an`. M3-S5 tests a squash-only lifecycle.

Linking a trunk commit back to the issue it resolved is the one thing file content cannot answer,
so `isu resolve` writes an `Isu-Resolves: <ID>` **commit trailer**. Recovery reads three sources
in order: the trailer, the **branch name** recorded by the merge, then the commit subject.

The middle tier is not redundant, and it does not depend on a second forge. GitHub's squash
commit message is a repository setting: set it to *pull request title*, or let a merge queue
compose the message, and the source commits' trailers never reach trunk. The subject is then the
pull request's title rather than anything `isu resolve` wrote, so the trailer and the subject can
both be useless on the same commit. Branch names are `isu/<ID>`, are recorded by the merge
whatever that setting says, and the importer's recovery pass scans for exactly that.

## The build

Ten milestones, fifty-one stories. Each is an epic under `issues/`, and the stories beneath it
carry their own briefs and their `**Done**` records — so the state of the build is
`isu board`, not a table somebody has to remember to edit.

| id | milestone | ships |
|---|---|---|
| M0 | Foundations | repo, CI, lint, test harness |
| M1 | Issue files | parse, serialise, validate, version |
| M2 | Git layer | fast load from any ref, from the working tree, and from trunk history |
| M3 | Derivation | statuses, epics, claims, contention |
| M4 | CLI | board, show, ready, new, claim, resolve, drop, comment, triage, field notes |
| M5 | Checks | `isu check`, hooks, GitHub Actions, dogfooding |
| M6 | TUI | `isu ui` |
| M7 | Importers | safe writes, GitHub Issues |
| M8 | Public website | content, landing page, docs, deploy |
| M9 | Release | goreleaser, brew, docs, v1.0.0 |

Sizing was estimated in pull requests, because the reviewer is the constraint and the compiler is
not:

| milestone | stories | shape |
|---|---|---|
| M0 Foundations | 4 | small, mostly config; M0-S4 is the one that matters |
| M1 Issue files | 5 | small, pure functions, heavy table tests |
| M2 Git layer | 5 | medium; M2-S2 is the trickiest parsing in the project |
| M3 Derivation | 5 | medium; this is the product, expect the most review time here |
| M4 CLI | 8 | medium; M4-S8 has no code and may generate several follow-ups |
| M5 Checks | 7 | small each, and highly parallel in principle |
| M6 TUI | 5 | medium; golden-frame tests are fiddly to stabilise |
| M7 Importers | 5 | large; M7-S4 is the biggest single PR in the plan |
| M8 Website | 3 | medium |
| M9 Release | 4 | small, except M9-S3 which is open-ended by design |

**Fifty-one stories, and at most fifty-one pull requests** — an upper bound rather than a
forecast, since related stories may travel together and M1 through M4 each arrived as a single
pull request. Grouping moves that number, not the work underneath it. The agent is not the
bottleneck.

Two stories were flagged as able to generate unplanned work and not to be scheduled tightly:
**M4-S8**, where real repositories got their say, and **M9-S3**, where hardening turns every
defect into a regression test first.

## The wall-clock budgets

`make perf` asserts them, and nothing else does. A budget on elapsed time is a claim about the
whole machine, and `go test ./...` runs `internal/cli`'s seventy seconds of git beside the
package being timed — which put this gate red on macOS three times before the measurement was
moved somewhere quiet. `make perf` runs the one package in one process and sets `ISU_PERF` to
say so, with `-v` so the number it measured is on the record of every run; the default suite
still runs those tests and logs the same figure, and the **process-count** assertions beside
them — the ones that actually prevent the regression — hold in every pass.

macOS carries a further factor-of-two allowance, because `macos-latest` is measured at 2.58× the
runner the budgets were taken on — 707 ms for `LoadRef` there against 274 ms here, and 3.66 s
for the board against 1.41 s. Those left the original numbers 2.12× and 1.64× of headroom on an
idle macOS runner, which is why both budgets carry it and not just the one that went red.
