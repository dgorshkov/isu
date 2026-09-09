# isu v1.0.0 — build plan

`isu` is an issue tracker with no database. Issues are folders inside the repo; a pull
request that fixes a bug also closes it, in the same diff. Written in Go, shipped as one
binary containing a CLI and a TUI.

This document is the build order. Work it top to bottom. A pull request carries one story, or
a group of related stories where §0 allows it. Do not skip ahead, do not merge your own work.

---

## 0. Working agreement

**Read this section before every session.**

- **One branch = one pull request. A pull request carries one story, or several related
  stories.** A single story is the default unit and the safest one; grouping is allowed, not
  required. Group only stories that are neighbours in this document, belong to the same
  milestone, and share one subject a reviewer can hold in their head at once — M1's five
  issue-file stories, or M4's eight commands, are the shape of it. Never group across a
  milestone boundary. Never group so much that one sitting cannot review it: a pull request
  nobody finishes reading is worse than the four they would have finished.
  Branch name is the story id, slugged: `isu/M3-S2-derive-epic-rollup`; a branch carrying
  several stories is named for the range and its subject: `isu/M1-S1-S5-issue-files`.
  Everything else in this document stays **per story** whatever the branch carries — its own
  red-then-green commits, its own tests, its own `**Done**` line — and the pull request
  description lists the story ids it closes.
- **TDD, visibly.** The failing test is its own commit and precedes the commit that makes it
  pass. Refactor after. A reviewer must be able to see red-then-green in the commit history.
  **CI gates the branch head, not every commit.** A Go test naming types that do not exist yet
  does not compile, so a per-commit gate would go red on every branch's first push, and a
  pipeline that is red by design is a pipeline people learn to ignore.
- **Never merge your own pull request.** Open it, fill the template, stop. Wait for review.
- **Stop at every milestone boundary.** After the last story of a milestone, open the PR and
  wait for explicit approval to begin the next milestone. Do not continue.
- **Commit format:** `M3-S2: derive epic rollup from children` — story id, colon, imperative
  summary. Body explains why, not what.
- **If a story needs a decision that isn't in this document, stop and ask.** Do not invent
  product behaviour. Write the question in the PR description.
- **If a test proves this document wrong, stop and say so.** The decisions here were validated
  on throwaway prototypes, not on this codebase. When a story's tests show the specified
  behaviour is wrong, incoherent or impossible, do not work around it and do not silently
  redesign. Open the pull request containing the failing test, the evidence, and a proposed
  amendment to this file. **A story that ends in a corrected PLAN.md and no implementation is
  a successful story.**
- **Every story leaves the tree green:** `go build ./...`, `go vet ./...`,
  `golangci-lint run`, `go test ./...` all pass, and coverage stays at or above the floors in
  the definition of done. The floors are the gate; there is no per-story ratchet on top of
  them, because a ratchet punishes honest deletion.
- **Name the reviewer before M0-S1.** Somewhere between a dozen and fifty-one pull requests
  arrive one at a time, depending on how much travels together, and none of them merge without a
  human. The schedule below is that person's calendar, not the agent's.
- **Mark it done in this file, in the same pull request.** The pull request that finishes a
  story also updates PLAN.md, or the story is not finished — and one carrying several stories
  does this for every one of them. Specifically:
  - append ` ✅` to the story heading, and add a `**Done**` line directly beneath it naming
    the pull request and the date it merged — `**Done** #1, 2026-08-24`;
  - update the **status** column of that story's milestone in the Milestones table;
  - when the last story of a milestone lands, mark the milestone heading ` ✅` too and set
    its row to `done`.

  Mark a story done only once its pull request has merged — not when the branch is pushed.
  Never mark a story that was skipped, deferred or partially built; say what is missing
  instead. This file is how a reviewer, and the next session, learn where the build actually
  is, so a stale PLAN.md is a defect like any other.

### Stack

| | |
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
released most recently, resolved fresh on every CI run — and the lint gate went red the day one of
those releases arrived incomplete. A gate that fails on a schedule nobody controls is the pipeline
§0 warns about, so the directive moved to 1.24 rather than the symptom being pinned around. It
moved again to **1.24.2** in M6, because three of the modules bubbletea and lipgloss bring declare
that and a module's directive must be at least its dependencies'. Still no `toolchain` line: the
directive is a minimum, and pinning a toolchain is the thing that went wrong in the first place.

Frontmatter is parsed by hand (M1-S1) because it is a flat key/value block and the round-trip
guarantee in M1-S3 is easier to hold without a YAML serialiser reformatting it. `.isu.yml` and
`source.yml` are real YAML and get a real parser.

Markdown is rendered for the terminal only, by `glamour`. **v1.0.0 ships no HTML**, so there
is no markdown-to-HTML pipeline and no sanitiser in the allowlist — see the out-of-scope list.

### Git access

**Shell out to the `git` binary. Do not use go-git.** Reasons, in order: the user's git
config, hooks, credential helpers and LFS all apply for free; plumbing commands give exact
control over the read path; and the fast load path below is only available through plumbing.
`git` is a hard runtime requirement and that is fine for a developer tool.

All git invocation goes through `internal/gitx`. Nothing outside that package may construct
a `git` command — **the test harness included**, which is where M2-S1 found the one
exception that would otherwise have been written into the rule on its first day.

### The read path — a hard performance requirement

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

`ref:path` costs a tree walk per lookup. Object ids skip it. M2-S5 enforces this with a
benchmark that fails the build if it regresses.

**The board is a different question and gets a different answer.** The figures above are one
ref; `isu board` reads trunk and every branch, and listing each ref's whole tree is a million
entries over 200 branches — measured at 11.4 s in M2-S5, against a 6 s budget. So trunk is
listed once, each other ref is *diffed* against it, and one `cat-file --batch` reads the
union of the blobs they name. Git compares trees by object id and skips the subtrees that
match, so a branch costs its own difference and not the repository. Measured: **1.42 s** for
5,000 issues over 200 branches.

---

## 1. The data model

Decided. Do not deviate without asking.

### Layout

```
issues/
  AR-7f3akq/
    README.md                 the issue
    repro.har                 attachments live with the issue
    comments/
      2026-08-24-support-01.md   append-only, one file per comment
```

`README.md` so that forges render the issue when you browse to the folder.

Comment files are `<date>-<author>-<nn>.md`. The two-digit sequence is not decoration: without
it, a second comment by the same author on the same day silently overwrites the first.

**Resolved issues stay in `issues/`.** There is no `fixed/` directory and no archive command
in v1. State lives in the file.

### Frontmatter

```yaml
---
schema: 1              required, integer, the on-disk format version
id: AR-7f3akq          required, must equal the folder name
title: Login retries   required, one line
type: bug              required: bug | story | chore | spike | epic
state: open            required except on epics: open | resolved | dropped
owner: dmitry          required: the human answerable for it
created: 2026-08-24    required, RFC 3339 date
priority: p2           optional: p0 | p1 | p2 | p3, default p2
parent: AR-40b1cc      optional, an id; must name an epic — checked in M5, not M1
blocked_by: AR-39ka2p  optional, comma-separated
repro: ...             required when type: bug
acceptance: ...        required when type: story
question: ...          required when type: spike
reason: ...            required when state: dropped
resolution: ...        required when state: dropped
---

Free-form markdown body.
```

`schema` exists so that v1.0.0 is not a format prison. A reader must refuse a `schema:` it
does not know, with a message naming the version it needs, rather than misparsing it. Adding
optional keys does not bump it; changing the meaning of an existing key does. M1-S5 proves
this is real rather than decorative.

`title` is what the board, the TUI list and every filter render. `created` is what issue age is
computed from — deriving it by walking history costs a git process per issue. `priority` is
what `isu ready` orders by. All three are load-bearing for stories in M4 and M6.

`resolution` is one of `wontfix | duplicate | works-as-intended | fixed-elsewhere` and sits
alongside the free-text `reason`. `dropped` alone cannot distinguish a duplicate from a
won't-fix, and the difference is the first thing anyone asks.

`owner` is the accountable human. It is set at triage and an agent must never change it —
see M5-S4. Who is *working* on an issue right now is a different question, answered by the
claiming branch, not by this field.

`parent` must name an issue of `type: epic`, and **that is a repository-level rule, not a
field-level one.** `Validate()` in M1-S2 takes one issue and nothing else, so it checks that
`parent` is shaped like an id and stops there; whether the id resolves, and whether what it
resolves to is an epic, is a structural check in M5-S2 that has the whole set loaded. The same
goes for `blocked_by`. Reading the row above as a rule M1 enforces is the mistake this
paragraph exists to prevent: it would require loading a second issue to validate the first,
which the milestone explicitly forbids.

### Type determines what closing it requires

| type | required field | resolving it requires |
|---|---|---|
| `bug` | `repro` | a change outside `issues/` |
| `story` | `acceptance` | a change outside `issues/` |
| `chore` | — | a change outside `issues/` |
| `spike` | `question` | a file in the issue's own folder that isn't `README.md` |
| `epic` | — | nothing — an epic has no `state:`; its status is the fold over its children |
| any | — | `state: dropped` requires `reason` and `resolution` |

An `epic` **must not** declare `state:`, and everything else **must**. Epic-ness is declared in
the file rather than inferred from who points at it, so validating one issue never requires
loading another, and no pull request of yours can invalidate a file someone else owns by adding
a `parent:` line to a third file.

### Derived statuses

Never stored. Computed from `state` read across refs. **The rows are ordered and the first
match wins** — several of them overlap, and without a stated precedence a merged issue whose
claiming branch was never deleted matches both `done` and `in progress`.

| status | rule |
|---|---|
| `done` | trunk has `state: resolved` |
| `dropped` | trunk has `state: dropped` |
| `awaiting triage` | folder exists on a branch, not on trunk |
| `in progress` | some branch has `state: resolved` where trunk has `open` |
| `reopened` | trunk has `state: open`, and some earlier trunk commit had `resolved` |
| `open` | on trunk with `state: open`, and no branch claims it |

Terminal trunk state beats every claim, so a finished issue reads `done` whether or not the
branch that claimed it was tidied up. `in progress` beats `reopened` because someone actively
re-fixing an issue needs to show as worked, not as merely broken again — but **reopened
survives as an annotation** on whatever status wins, so the fact is never lost.

**`reopened` reads every earlier trunk commit, and a deletion in between does not break the
chain.** Issues are files, so somebody can `git rm` a folder and commit it — there is no
command for that, but nothing prevents it either — and an id can come back afterwards, whether
by reverting that commit or by re-running an import, since imported issues keep their source
key verbatim and so repeat exactly. Decided in #8, after M3-S4 asked: an id is permanent from
creation, so the same id is the same issue, and trunk did resolve it once. It reads `reopened`.
The rejected reading was that a removal ends the file's story, the way M2-S4 treats a rename;
the difference is that a rename produces a *different* id and this does not.

`in progress` has exactly one source, and it is the claim itself: `isu claim` writes
`state: resolved` on branch `isu/<ID>` before any work starts (see Claims below). So a claimed
issue and an issue somebody resolved on a branch without claiming are the same observable fact,
read the same way — there is no advisory side-channel to reconcile against what the branches
say, because there is no second signal.

**A branch that exists without that flip is deliberately not `in progress`.** It is somebody's
work on a branch, and until they say so by claiming, the board does not speak for them. That is
also what makes `isu unclaim` mean anything: it flips the state back and leaves the branch
standing.

Annotations on `in progress`: **claimant** (author of the commit that flipped the state, which
is the first commit on the claiming branch), **age** (that commit's author date), **stale**
(older than 7 days), **contended** (two branches claiming the same issue).

An issue of `type: epic` takes its status from a fold over its children. All children terminal
→ resolved, unless all are dropped → dropped. Otherwise open. An epic with no children is a
check failure, not a status.

### IDs

`<PREFIX>-<token>`, prefix from `.isu.yml`. The token is the first thirty bits of
`sha256(title ‖ owner ‖ created ‖ 8 bytes from crypto/rand)`, rendered as six characters of
Crockford base32 — lowercase, with `i`, `l`, `o` and `u` excluded from the alphabet so nothing
is ambiguous read aloud or typed from a screenshot.

**‖ is a NUL byte, not concatenation.** Running the fields together makes the boundary between
two of them imaginary, so a title ending in the owner's name hashes the same as a shorter
title and a longer owner. `created` is fed in as `YYYY-MM-DD`.

The prefix is part of every issue's **folder name**, so it has to be a legal one: `.isu.yml`
validation holds it to the same character set an id is held to.

Allocation needs no coordination, no counter and no network round trip, which is the point:
sequential `max + 1` cannot see issues sitting on unmerged `report/*` branches, so under any
real load it hands out the same number twice.

`isu new` regenerates on a collision with any id it can see (trunk plus local refs). A
duplicate that survives that — two clones creating an issue at the same moment — remains a hard
CI failure, and is now rare enough to be an incident rather than a routine.

**There is no `isu renumber`, and ids are never rewritten.** An id is permanent from creation:
that is what lets `blocked_by` on one branch keep pointing at the right issue while a hundred
other branches are in flight. **Imported issues keep their source key verbatim** as their id —
`PROJ-1234` is a valid id, and the format above governs generation, not validation.

**A source key that cannot be an id is formed into one, and that is the only case in which an
import does not keep the key.** GitHub numbers issues per repository, from the same sequence it
numbers pull requests, so the key anybody actually writes is `#1234` or `owner/repo#1234` and
neither is a folder name: `#` and `/` are not in the id character set. M7-S4 therefore writes
`<PREFIX>-1234`, taking the prefix from `.isu.yml` and keeping the number, which is the half a
human recognises and the half every link contains; `owner/repo#1234` and the issue's URL go to
`source.yml`, so nothing about the source is lost. `--id-prefix` overrides the prefix for a team
importing into a tracker whose own prefix means something else. Milestones become epics and are
numbered from a second sequence, so they take `<PREFIX>-M<number>` — `ISU-M3` and `ISU-3` are
different issues, and a milestone's title is renameable where its number is not. Cross-repo
issues are out of scope, so one import cannot collide with itself; two imports into one tracker
can, and M7-S4 refuses rather than overwriting.

### Configuration

`.isu.yml` sits at the repository root. This is the whole schema; stories below may not invent
keys, and an unknown key is a validation error rather than a silent no-op.

| key | type | default | meaning |
|---|---|---|---|
| `prefix` | string | — | required; the issue id prefix, and a legal folder name |
| `agents` | list of strings | empty | commit authors treated as agents by M5-S4 |
| `direct_triage` | bool | `false` | allow `isu triage --push` to write straight to trunk |
| `stale_days` | int | `7` | a claim older than this is stale |
| `fetch_warn_hours` | int | `24` | warn when the newest remote ref is older than this |
| `attachment_max_bytes` | int | `524288` | per-attachment cap enforced by M5-S2 |

Every one of these is read by a story below, so the file's schema is validated in M1-S4 rather
than discovered a milestone at a time.

**This question was open until M4-S1 and is now answered: `isu init` writes the file.** Every
story in this document *reads* `.isu.yml` and no story wrote one, so a person adopting isu in an
existing repository had to hand-write it before any command worked, and `prefix` is required, so
an absent file is a hard failure rather than a degraded mode. Of the three answers offered — a
small `isu init` story in M4, a copy-and-paste block in M8's docs, an entry on the out-of-scope
list — the first is the only one that makes the first thing somebody types after installing isu
do something. M4-S8 confirmed it was worth making: every command against all three real
repositories failed on the missing file before anything else could be measured.

`isu init` writes `prefix` and nothing else. Every other key has a default, and a configuration
file full of the defaults is a file nobody can tell they have changed.

### Claims

**This section was rewritten after the design it described was tested and found to be paying
for something it did not need.** The previous version claimed through a parentless commit at
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

`--force-with-lease=refs/heads/isu/<ID>:` — the third mechanism named at the end of this section
— does not close it either: git short-circuits on "up to date" before the lease is evaluated.
Measured, both of them.

What closes it is making this section's own sentence true. Atomicity comes from committing
something nobody else can have committed, so `isu claim` writes an **`Isu-Claim:` trailer holding
a fresh random value** on the claim commit. The second push is then rejected as a
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

**Pushing a bare branch at the trunk tip would not work**, and that is the trap the earlier
draft was written to avoid: git accepts it as a no-op fast-forward and tells both claimants
they succeeded, measured as `[new branch]` for the first and `Everything up-to-date` for the
second, both exit 0. Atomicity comes from committing something nobody else can have committed,
not from the ref namespace — a claim ref was one way to get that, and a commit that flips the
state is another. (`--force-with-lease=refs/heads/isu/<ID>:` with an empty expected value is a
third, rejecting the second push with `stale info`. It is not needed here and is not used.)

**The claim writes `resolved` before the work is done, and that is deliberate.** A branch is a
proposal, not a fact: `state: resolved` on `isu/<ID>` says *this branch intends to resolve this
issue*, and it becomes a fact about the repository when the branch merges. Trunk is where state
is true. Until then the only thing that reads it is a board that renders it as `in progress`.

**Claiming does not get you past the evidence check.** M5-S3 requires a change outside
`issues/` to resolve a bug, a story or a chore, and an artifact in the issue's own folder to
resolve a spike. A claim changes one line of one README and nothing else, so a branch that
claimed an issue and did no work fails that check exactly as it should — the check reads the
diff, not the field. The earlier draft's objection to writing `resolved` early, that every
claim would then look like a resolution, is not true for this reason.

**Claimant and claim time are the first commit on the claiming branch.**
`git log <trunk>..isu/<ID> --reverse` is the commit that flipped the state — the **first**
record of that walk; its author is the claimant and its author date is the claim time. That is
one git process per claiming branch — linear in refs, which is what the board already costs
(M2-S5) — where a dedicated claim ref would have made it free. It is the one thing this design
pays more for. The branch *tip* is free from `for-each-ref` and is the **wrong** answer: it
moves every time the claimant pushes more work, so a claim would never age and `stale_days`
would never fire.

**This paragraph said `--reverse --max-count=1` until M3-S3, and that pair returns the tip.**
Git applies the limit during the walk, which starts at the tip, and reverses what survived it;
one commit reversed is that same commit. Measured on a branch of three commits over trunk:

```
git log --reverse --max-count=1 main..topic  ->  third on branch
git log --reverse             main..topic  ->  first on branch, second on branch, third on branch
```

So the spelling above walks the range and takes the first record, which costs a claiming
branch's own commits rather than a constant. That is affordable — a claiming branch is a few
commits, not a repository — and it is correct, which the pair is not at any price. The
correction is the same in M3-S3 below, and `TestLoadFirstCommitsTakesTheFirstCommitAndNotTheTip`
asserts the commit id rather than only the date, so a lookup that is accidentally right on a
one-commit branch cannot pass it.

`isu unclaim` flips the state back to `open` on the claiming branch and pushes. **It does not
delete the branch** — a one-word command must not throw away work. What it releases is the
claim; what it leaves is an ordinary branch the board says nothing about. Claims are advisory
for humans and binding for agents.

**A claim is released when the work lands, and there is no sweep.** Terminal trunk state wins
the precedence above, so a merged issue reads `done` whatever its branch still says, and the
branch is the forge's to delete on merge. Nothing outside `refs/heads/*` accumulates, so
nothing has to remember to tidy it — where the earlier draft needed a CI job in M5-S6 to delete
`refs/claims/<ID>`, without which M5-S5 would report a stale claim on every finished issue for
the life of the repository.

**Nothing here pushes outside `refs/heads/*`.** The remotes that refuse other namespaces are
not a special case, there is no `--no-claim` degraded mode to build, and a rejected push is
unambiguous: on a branch it is a lost race and never a policy refusal. The earlier draft paid
for all three — it needed a mode nobody would exercise, and it needed `isu claim` to tell two
identical-looking git failures apart and explain which one had happened.

### Squash-merge safety

Post-merge questions are answered from **file content at trunk commits**, never from commit
metadata. Squash collapses authorship; it does not touch the file. Every derivation that
reads history must read blob content, not `%an`. M3-S5 tests a squash-only lifecycle.

Linking a trunk commit back to the issue it resolved is the one thing file content cannot
answer, so `isu resolve` writes an `Isu-Resolves: <ID>` **commit trailer**. Recovery reads three
sources in order: the trailer, the **branch name** recorded by the merge, then the commit
subject.

The middle tier is not redundant, and it does not depend on a second forge. GitHub's squash
commit message is a repository setting: set it to *pull request title*, or let a merge queue
compose the message, and the source commits' trailers never reach trunk. The subject is then
the pull request's title rather than anything `isu resolve` wrote, so the trailer and the
subject can both be useless on the same commit. Branch names are `isu/<ID>`, are recorded by
the merge whatever that setting says, and M7-S3 already scans for exactly that.

---

## Milestones

Ten milestones. Stop for review at the end of each.

| | milestone | ships | status |
|---|---|---|---|
| M0 | Foundations | repo, CI, lint, test harness | done |
| M1 | Issue files | parse, serialise, validate, version | done |
| M2 | Git layer | fast load from any ref, from the working tree, and from trunk history | done |
| M3 | Derivation | statuses, epics, claims, contention | done |
| M4 | CLI | board, show, ready, new, claim, resolve, drop, comment, triage, field notes | done |
| M5 | Checks | `isu check`, hooks, GitHub Actions, dogfooding | done |
| M6 | TUI | `isu ui` | done |
| M7 | Importers | safe writes, GitHub Issues | not started |
| M8 | Public website | content, landing page, docs, deploy | done |
| M9 | Release | goreleaser, brew, docs, v1.0.0 | not started |

### Sizing

Estimated in pull requests, because the reviewer is the constraint and the compiler is not.

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
forecast, since §0 lets related stories travel together and M1 through M4 each arrived as a
single pull request. At three reviewed per day that bound is roughly four weeks; at one per day,
roughly eleven. Grouping moves that number, not the work underneath it. The agent is not the
bottleneck — plan your own calendar, not its.

Two stories can generate unplanned work and should not be scheduled tightly: **M4-S8**,
where real repositories get their say, and **M9-S3**, where hardening turns every defect into
a regression test first.

---

# M0 · Foundations ✅

**Status** done — all four stories landed. The milestone boundary rule in §0 applies
here: M1 does not start without explicit approval.

### M0-S1 · Repository skeleton ✅
**Done** #1, 2026-08-24. Module path confirmed as `github.com/dgorshkov/isu`.
**Branch** `isu/M0-S1-skeleton`
**Decide first** the module path. `github.com/dgorshkov/isu` is a placeholder — confirm the
real one before the first commit, because changing it later rewrites every import in the tree.
**Build** `go.mod`, `cmd/isu/main.go` printing version, Apache-2.0
`LICENSE`, `README.md` with one paragraph and the install line, `.gitignore`, `.isu.yml`
carrying `prefix: ISU`.
**Tests first** `TestVersionCommand` asserts `isu --version` prints a semver string.
**Done when** `go run ./cmd/isu --version` works and the tree is green.

### M0-S2 · Lint, vet, coverage gate ✅
**Done** #4, 2026-08-25. The profile is built with `-coverpkg=./...`, without which a
package carrying no test file of its own is absent from the profile and raises the
average by being untested.

**Amended in #8: the overall floor went from 85% to 99%.** The figure in the story below is the
one this shipped with; measuring the tree found 85% was not a gate at all, since the actual
number was 96.9% and twelve points could have gone missing unnoticed. Still two floors, still
across `./...` — the reasoning, including why the test harness is counted rather than exempted,
is in the definition of done and in `coverage.sh`. The test that isolates the package floor
from the overall one now lowers the overall floor through the environment rather than relying
on a fixture's size to do it, which is what those knobs were put there for.
**Branch** `isu/M0-S2-quality-gates`
**Build** `.golangci.yml` (errcheck, govet, staticcheck, revive, gofumpt), a `Makefile` with
`make test lint cover`, and a coverage script enforcing **both** floors from the definition of
done: **85% across `./...`** — the whole module, `cmd/` included, not just `./internal/...` —
and **100% for `internal/model`** once that package exists. A gate that measures a subset of
the tree is not the gate §0 says it is.
**Tests first** a test that shells the coverage script against a fixture below the overall
threshold and asserts non-zero exit; a second fixture that clears 85% overall but leaves
`internal/model` under 100% and must also exit non-zero; a third asserting the per-package
floor is skipped, not failed, while `internal/model` does not yet exist.
**Done when** `make lint` and `make cover` both pass locally, and neither floor can be met by
a tree that violates the other.

### M0-S3 · CI ✅
**Done** #4, 2026-08-25. **This story was amended as it was built.** It read *CI on both
forges* and shipped a `.gitlab-ci.yml` alongside the Actions workflow; the project lives on
GitHub only, so the GitLab pipeline was deleted in the same pull request and every other
reference to a second forge in this file was corrected with it — see *Out of scope* below.
**Branch** `isu/M0-S3-ci`
**Build** `.github/workflows/ci.yml`, running build, vet, lint, test and coverage on Linux and
macOS. It calls `make` targets — no logic in YAML, so every gate can be run before pushing.
**Tests first** `TestMakefileTargetsExist` parses the Makefile and asserts every target the CI
file references is defined. This is what stops a gate existing only inside YAML.
**Done when** the pipeline is green.

### M0-S4 · The git test harness ✅
**Done** #4, 2026-08-25. Commits by a second author are not in the harness yet; they
arrive with M3-S3, which is where contention needs them. Claim refs were listed here too
and are no longer a thing the harness will ever need — a claim is a branch, and the
harness already builds branches.
**Branch** `isu/M0-S4-gittest`
**Why** Every meaningful test in this project builds a real repository. Getting this helper
right early is the difference between fast tests and a swamp.
**Build** `internal/gittest` with a fluent builder: `New(t)`, `.Issue(id, opts...)`,
`.Commit(msg)`, `.Branch(name)`, `.Checkout(name)`, `.Merge(branch)`,
`.SquashMerge(branch, subject)`, `.Revert(ref)`, `.WithRemote()`, `.DetachRemote()`,
`.File(path, content)`, `.Backdate(days)`. Repos go in `t.TempDir()` and clean themselves up.
**Tests first** the harness tests itself: build a repo, assert the resulting `git log`,
`ls-tree` and branch topology are exactly as scripted.
**Done when** a five-line test can produce a repo with two branches and a squash merge.

---

# M1 · Issue files ✅

**Status** done — all five stories landed in one pull request rather than five. That was
asked for explicitly at the time, and §0 now allows it outright: five neighbouring stories on
one subject are the group it describes. The milestone boundary rule still applies: M2
does not start without explicit approval.

### M1-S1 · Frontmatter parser ✅
**Done** #5, 2026-08-26. Blank lines inside the block are preserved and comments are not part
of the format — this is not YAML, so a line with no colon is a parse error. `Document.Set`
flattens line breaks in a value to spaces, because the format has no folding and a value
carrying one would write a file that does not parse back.
**Branch** `isu/M1-S1-frontmatter`
**Build** `internal/issue`: parse `---` delimited key/value frontmatter plus body. Unknown
keys are preserved verbatim on round-trip. Parsing never panics on malformed input; it
returns a typed error with line number.
**Tests first** table-driven: valid, missing close delimiter, duplicate key, empty file, CRLF
line endings, unicode values, a 1 MB body. Plus a round-trip property test: parse → serialise
→ parse yields an identical struct.
**Done when** the round-trip test passes on every fixture in `testdata/issues/`.

### M1-S2 · The Issue type and its schema ✅
**Done** #5, 2026-08-26. Built in `internal/issue`, not `internal/model` — the latter is
M3-S1's derivation package, and the coverage gate's own test wrongly said it arrived here.
`priority` is decoded as written and defaulted at `EffectivePriority()` rather than in the
struct, or `Encode` would write `priority: p2` into a file whose author never typed it and
M1-S3's zero-diff property would be gone. `parent:` is checked for shape and not for what it
names: the frontmatter table read as though M1 enforced it, which would mean loading a second
issue to validate the first, so section 1 now says out loud that it is M5-S2's.
**Branch** `isu/M1-S2-schema`
**Build** the `Issue` struct, the `Type` and `State` enums, and `Validate()` implementing the
required-field table from section 1. Errors accumulate — return all problems, not the first.
`Validate()` takes one issue and nothing else: no child index, no sibling lookup, no repo.
**Tests first** one case per row of the type table, plus: id not matching folder, missing
title, missing or malformed `created`, unknown type, unknown state, unknown priority, dropped
without `reason`, dropped without `resolution`, **dropped with a `resolution` outside the
enum**, bug without repro, **an epic declaring `state:`**, and **a non-epic omitting it**.
**Done when** `Validate()` output is stable, sorted and human-readable, and the epic cases pass
without the function ever seeing a second issue.

### M1-S3 · Reading and writing an issue folder ✅
**Done** #5, 2026-08-26. The zero-diff test runs against every fixture in `testdata/issues/`
and asserts it with `git diff --exit-code` on a real repository. Two things in a folder are
refused rather than absorbed: a plain file named `comments`, and a directory inside
`comments/`. `Write` touches README.md only.
**Branch** `isu/M1-S3-folder-io`
**Build** load an issue from `issues/<ID>/`, listing attachments and `comments/`. Write an
issue back, preserving unknown keys and body byte-for-byte where unchanged.
**Tests first** a folder with attachments and three comments loads with all of them; writing
an unmodified issue produces a zero-length diff (assert with `git diff --exit-code`).
**Done when** the zero-diff test passes. This property matters more than it looks: it is what
keeps pull requests readable.

### M1-S4 · ID generation and `.isu.yml` ✅
**Done** #5, 2026-08-26. **The `NewID` signature below was corrected as this story was
built** — it read `NewID(title, owner, created)`, which cannot return an id because it is
never told the prefix. The separator in the id hash was pinned to a NUL byte in section 1 for
the same reason: it was written as `‖` and never defined. "Zero reads" is proven in two halves
— the id is identical inside a 5,000-issue repository and in an empty directory, and identical
again with the working directory deleted out from under the process. Config is read and
validated; **nothing writes `.isu.yml`** — see the open question under Configuration.
**Branch** `isu/M1-S4-ids`
**Build** config loading and validation against the `.isu.yml` table in section 1, plus
`NewID(prefix, title, owner, created)` implementing the token scheme, and
`config.Config.NewID(title, owner, created)` over it for callers that already hold the
configuration. **An id is `<PREFIX>-<token>`, so the generator has to be told the prefix**;
earlier drafts of this line omitted it and described a function that cannot return an id.
**`NewID` is pure** — it reads nothing, knows about no other issue, and cannot detect a
collision, because detecting one means loading the repository and the git layer does not exist
until M2. No counter, no scan for a maximum, no `isu renumber` — ids are permanent from
creation.
**Tests first** the same inputs plus different random bytes give different ids; the alphabet
never emits `i`, `l`, `o` or `u`; `NewID` issues zero reads against a 5,000-issue fixture; an
imported `PROJ-1234` validates as an id even though nothing would ever generate it; every key
in the config table round-trips with its default applied, an unknown key is refused, and a bad
value is refused with the key named.
**Done when** generating an id needs neither the network nor a read of `issues/`. Collision
regeneration is deliberately **not** here — it needs the loaded repo, so it belongs to `isu new`
in M4-S3.

### M1-S5 · Schema version and migration ✅
**Done** #5, 2026-08-26. **Two lines below were corrected as this story was built**: the
error message cannot name "the version of `isu` that understands it", because that build does
not exist; and "byte-identical version 1 file" did not say identical to what. Gating runs in
both directions — an older `schema:` is refused as needing migration rather than read on a
guess.
**Branch** `isu/M1-S5-schema-version`
**Why** `schema:` is the promise that v1.0.0 is not a format prison, and an untested promise is
decoration. This story is what makes the field real, and it is cheap now and expensive after
people have repositories.
**Build** version gating in the reader, **in both directions**: a known `schema:` loads, a
newer one is refused, and an older one is refused as needing migration rather than read on a
guess. The message names the version it found and the version this build reads. **It cannot
name "the version of `isu` that understands it"** — that build does not exist yet and nothing
in this repository can know its number — so it says what to do instead: *upgrade to a build of
isu that reads version N*. Plus a `Migration` interface and the registry that dispatches on
version, with zero migrations registered.
**Tests first** `schema: 1` loads; `schema: 2` is refused and the error names both versions;
`schema:` missing is refused; `schema: banana` is refused with a parse error rather than a
panic; a registered no-op migration from a fixture at version 0 produces a version 1 file
**byte-identical to the fixture apart from the `schema:` line itself** — unknown keys, blank
lines inside the block, spacing and the body all untouched. The registry writes that line
after each migration returns, so a migration with nothing to do is an empty method body and
cannot forget to bump the version or bump it twice.
**Done when** a `schema: 2` repository fails with an error a human can act on, proven by a test
rather than by inspection.

---

# M2 · Git layer ✅

**Status** done — all five stories landed in one pull request rather than five. That was
asked for explicitly, as it was for M1, and §0 now allows it outright. What §0 still asks for
is the judgement, milestone by milestone: group what one reviewer reads in one sitting, not
whatever happens to be adjacent. The milestone boundary rule still applies: M3 does not start
without explicit approval.

**Claim refs are not loaded yet.** M3-S1 names them among its inputs, but M0-S4's note
already defers them to M3-S3, "which is where what they mean is decided", and M2 did not
skip ahead of that. `gitx.ForEachRef` lists `refs/claims/` today and has a test that does;
`repo.Board` grows a field for them in M3-S3, which is the loader growing rather than
derivation reaching for git — §M3-S1's rule, kept.

**M3-S3 decided, and the answer was that there is nothing to load.** The Claims section was
rewritten in #7 to claim by branching and flipping the state, so no `refs/claims/` namespace
exists to read and the field this paragraph promised was never added. What M3-S3 did add to
`repo.Board` is `Changed`, for a different reason entirely — see M3-S1.

### M2-S1 · `internal/gitx` ✅
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
are the whole reason §0 shells out.

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

### M2-S2 · Load every issue from a ref ✅
**Done** #6, 2026-08-26. **The return type below was corrected as this story was built.** It
reads `map[string]Issue`, and it cannot be one: M2-S3 requires a half-written issue to be
reported rather than fatal, and a map of the issues that loaded has nowhere to say which
ones did not. So both loaders return a `Set`, carrying `Issues` and `Broken`. Decoding stays
separate from validating — a bug with no repro is in `Issues`, because `isu check` cannot
report what the loader refused to hand it, and only a file that could not be parsed at all
is in `Broken`. Issues are keyed by **folder name** even where the frontmatter's `id`
disagrees, because that is what every `parent:` and `blocked_by:` in a repository points at
and `Validate` already reports the difference. Two cases the story's list does not name and
the read path forces: two byte-identical issue files are one blob, so the object-id mapping
is one-to-many or one of them is lost; and an unborn HEAD loads as an empty board while a
named ref that does not resolve stays an error, or a typo renders as a repository with no
issues in it. Comments and attachments are not read at a ref — nothing on the board or in a
derivation reads them.
**Branch** `isu/M2-S2-load`
**Build** `repo.LoadRef(ref) map[string]Issue` using the mandated read path: `ls-tree -r` for
object ids, one `cat-file --batch` fed object ids, stream-parse the output.
**Tests first** load from trunk, from a branch, from a detached SHA, from an empty repo;
issues present on one ref and absent on another; a blob containing the batch delimiter
sequence in its body (this will break a naive parser — write that test first).
**Done when** loading is correct on all of the above.

### M2-S3 · Load from the working tree ✅
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

### M2-S4 · The trunk history index ✅
**Done** #6, 2026-08-26. **The first test case below was corrected as this story was
built.** An issue resolved and then reverted holds **three** states at trunk, not two: it
was open when it was created. The case beside it — an issue never resolved yields one entry
— is that same creation counted, and under any single rule the two numbers cannot both be
right. The rule that keeps the second is *one entry per trunk commit at which the value
changed, its first appearance included*, and collapsing the commits that changed something
else is what keeps the sequence about states rather than about typo fixes.
`--first-parent` is load-bearing and the story does not say so. A merge commit shows no
diff of its own, and under a pathspec git simplifies it away and reports the change at the
*branch* commit — a commit that was never on trunk, carrying the date the work was written
rather than the date it landed. The date recorded is the committer date for the same
reason. A blob that does not parse contributes no state and does not stop the walk, and
that is told apart from an epic — which parses and declares no state — by the lookup rather
than by the value, since both would read as the empty string.
**Branch** `isu/M2-S4-history-index`
**Why** `reopened` is one of the six statuses, and it is the only one that cannot be answered
from the current content of any ref. Walking history per issue inside the derivation package
would make M3-S1's "no git calls inside" rule a lie the moment M3-S4 lands. So the walk happens
here, in the git layer, and derivation stays pure over what it is handed.
**Build** `repo.LoadHistory(trunk) map[string][]StateAt` — for each issue, the ordered sequence
of `state` values its file has held at trunk, with the commit and date of each change. One pass
over `git log --format` plus one `cat-file --batch` fed object ids, in the style of M2-S2.
Never `%an`: the value comes from blob content.
**Tests first** an issue resolved then reverted yields two entries in order; an issue never
resolved yields one; an issue whose file was renamed is not followed (documented limitation,
asserted); a squash-only history yields the same sequence as a merge-commit history for the
same logical changes; process count stays constant as the number of issues grows.
**Done when** `LoadHistory` answers reopen for a 5,000-issue repo without a git process per
issue.

### M2-S5 · The performance gate ✅
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
rather than a model, but it is bracketed above by every slower algorithm §0 measured:
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
that grows linearly in refs and not at all in issues. The 0.6 s in section 0 was measured on a
single ref; it is not a number the product's main operation can be held to.
**Done when** both gates pass in CI on the slowest runner.

---

# M3 · Derivation ✅

**Status** done — all five stories landed in one pull request rather than five. That was asked
for explicitly, as it was for M1 and M2, and §0 now allows it outright. M3 is where the size of
a group starts to cost something: this milestone is the product, and it drew more review time
than anything before it. The milestone boundary rule applies as ever — M4 does not
start without explicit approval.

**Two corrections and one question came out of the tests**, and all three are recorded where
the decision was made rather than only here: the claim lookup in section 1 and M3-S3 returned
the branch tip and is corrected above; the merge-then-revert case reads a different status
depending on whether the branch was deleted, which M3-S4's test list did not expect; and an
issue deleted at trunk and reported again is a reopen, which M3-S4 asked about and section 1
now answers. Nothing in M3 is left open.

**Claim refs are not loaded, and now never will be.** M2's note deferred them to M3-S3, "which
is where what they mean is decided" — and what they mean is nothing: the Claims section was
rewritten in #7 to claim by branching and flipping the state, so there is no `refs/claims/`
namespace to read. `gitx.ForEachRef` can still list one, which is a general wrapper doing its
job rather than a loose end.

### M3-S1 · Status derivation ✅
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
the six statuses from the table in section 1. **Pure function over loaded inputs — no git calls
inside, and no exceptions to that rule later.** Everything a status needs is loaded by M2 and
passed in; if a future status needs something else, the loader grows, not this package.
**Tests first** one test per status, then the transitions between them. Include: issue on two
branches, issue on a branch identical to trunk, branch deleted after merge, a branch that
resolves an issue trunk has already resolved, and **a branch that exists without flipping the
state, which is not a claim and must read `open`**.
**Done when** the status function has 100% branch coverage. This package is the product; it
gets a higher bar than the rest.

### M3-S2 · Epic rollup ✅
**Done** #8, 2026-08-27. The fold marks each epic with one of three states — not started,
part-way through, finished — and meeting a part-way one is the cycle: the parent chain came
back round to where it started, so the answer is a value rather than one more stack frame. An
epic that is its own parent is the one-node version, and it is the case that blew a stack
during prototyping. Every child is folded even after the first unfinished one, because stopping
early is the same answer for *this* epic and a different one for the board — an epic further
down that nothing else points at would keep whatever status the walk happened to leave it with.
One case the story does not name: an epic reported on a branch and not yet on trunk reads
`awaiting triage` rather than folding, because a folder trunk has never seen is a report
whatever type it declares. The gate is met with room: every fifth issue is an epic, so 5,000
issues is **1,000** epics against the story's 100, and deriving the whole board takes **5.2 ms**
against a 50 ms budget — 10.4 ms when the same measurement was taken over a board that had been
read out of a repository, where the issues arrive spread across memory rather than allocated in
one pass. **The fixture is built in memory rather than generated as a repository**,
and that is not a shortcut: this story measures a fold over issues that are already loaded, and
generating 5,000 issue folders to read them back is half a minute of git this test does not
time. It is also half a minute spent on a CI runner that is at that moment timing M2-S5's read
path in another process — which is exactly how it made that gate fail twice before anybody
noticed the two were fighting. See M2-S5.
**Branch** `isu/M3-S2-epics`
**Build** children indexed once per load, not scanned per parent. Fold child states into every
`type: epic`. Cycle-safe: a parent cycle must return a value, never recurse forever.
**Tests first** rollup with mixed children; all dropped; nested epics three deep; **an epic
that is its own parent**; a two-node parent cycle; an epic with no children; a `parent:` naming
an issue that is not an epic. The self-parent case blew a stack during prototyping — write it
before the implementation.
**Done when** a 5,000-issue fixture rolls up 100 epics in under 50 ms.

### M3-S3 · Claimant, age, staleness, contention ✅
**Done** #8, 2026-08-27. **The lookup below was corrected as this story was built**, and the
correction is written up in section 1: `--reverse --max-count=1` returns the branch tip, which
that section calls the wrong answer in the paragraph above the one that spelled it. The range
is walked and its first record taken instead. The split between the two packages is the other
thing this story settled: *which* refs claim is a pure question about what was already loaded,
so `model.ClaimRefs` answers it, and *who* claimed costs a git process each, so
`repo.LoadFirstCommits` does that over the refs it named. Asking the loader to work both out
would have made it walk every branch in the repository. A ref with no first commit — nothing
ahead of trunk — is a lookup with no answer rather than a failure, and a claim whose first
commit was not looked up is still a claim: the file is the claim, and the lookup only names who
made it, so dropping the row would hide a claim to protect an annotation. **A zero
`Input.Config` means the default rather than zero days**, added after review: `StaleAfter()` of
zero makes every claim stale the instant it is made, and `Derive` already defaulted the sibling
zero value `Now`. A zero value that silently flags every row on the board as abandoned is a
footgun, and documenting it is not as good as not having it.
**Branch** `isu/M3-S3-claims`
**Build** a claim is a branch whose issue file says `state: resolved` where trunk says `open`.
The claimant is the author of the **first commit on that branch** and the claim time is its
author date; staleness is that date against `stale_days`. An issue is contended when more than
one branch claims it.
**The lookup belongs to the loader, not to this package.** `internal/repo` grows a call that
runs `git log <trunk>..<branch> --reverse` for each claiming branch and takes the first record,
handing the result over — one process per claiming branch, linear in refs like everything else
the board does. M3-S1's rule that derivation never calls git has no exceptions, and this is the
first story that would have been tempted to make one.
**Tests first** a single claim names the claimant and the date the state was flipped, **not the
branch tip's date** — assert with more work pushed on top, which moves the tip and must not
move the claim; two claiming branches by different authors read as contended with both named; a
claim backdated 11 days reads as stale; a branch that exists without the state flip is not a
claim; unclaim — the state flipped back to `open`, branch still standing — stops reading as
in progress; a claiming branch deleted after merge leaves the issue reading `done` from trunk.
**Done when** contention and staleness come out of what M2 already loaded plus one log per
claiming branch, and nothing in `internal/model` runs git.

### M3-S4 · Reopen detection ✅
**Done** #8, 2026-08-27. **The fold itself landed with M3-S1**, because the status table has a
`reopened` row and that story owns the table; this story is the case matrix over it and the
purity tests, and it found two things the story's list does not expect.

**Merge-then-revert does not read the same with the branch deleted and without it.** The list
below asks for the three cases "and the same three with the branch deleted after merge", which
reads as though the answers match. They do not, and the table is why: a branch left standing
through a revert still says `resolved` where trunk now says `open`, which is a claim by the
only definition of one there is, and `in progress` beats `reopened`. So it reads `reopened`
with the branch gone and `in progress` with it there. Nothing is lost — the annotation survives
either way — and the board is saying that somebody's branch disagrees with trunk, which is
exactly the situation.

**A deletion in the middle of an issue's history does not break the reopen chain — asked here,
answered in #8.** An issue whose folder was deleted at trunk and later written again holds a
removal between its states. The rule as the table states it makes that a reopen; M2-S4's
reading of a rename would have made it a different issue's history. The rule as written stands:
an id is permanent from creation, so the same id is the same issue, and trunk did resolve it
once. Section 1 now says so where the rule lives, rather than only here.

Two things the list does name and are worth keeping visible: `dropped` at an earlier commit is
deliberately *not* a reopen, because the table names `resolved` and undropping is a triage
decision rather than a fix that did not hold; and the purity rule got three tests rather than
one grep — what this package may import, that it may not name a `*repo.Repo`, and that a whole
derivation moves the repository's process counter by zero.
**Branch** `isu/M3-S4-reopen`
**Build** a **pure fold over the M2-S4 history index**: if any earlier entry was
`state: resolved` and the latest is `open`, the status is `reopened`. No git in this package —
that is the whole reason M2-S4 exists.
**Tests first** merge then revert reads as reopened; merge, revert, re-fix reads as done;
never-resolved reads as open; **and the same three with the branch deleted after merge**. Plus
a test asserting this package spawns no processes, in the style of the M2-S1 grep test.
**Done when** reopen is correct with zero branches left in the repo.

### M3-S5 · Squash-merge lifecycle ✅
**Done** #8, 2026-08-27. The lifecycle passes without changing M3-S1..S4, which is the result
the story was written to get. **"Build nothing new" is wrong by one function**, and the story's
own test list is what makes it wrong: it asks for a trailer recovery and a subject fallback,
and neither existed. `model.Resolves` is that function, and it reads two of section 1's three
tiers — the two that are in a commit message. The middle tier, the branch name the merge
recorded, is not in one: it needs the merge's own refs, so it belongs to whatever loads them,
and M7-S3 already scans for it.

**The subject tier is anchored on the repository's `prefix`, and the anchor is the whole of its
safety.** An id is letters, digits, a hyphen, an underscore and a full stop — because an
imported issue keeps its source key verbatim — so every word of an English sentence is a legal
id, and an unanchored scan would return the first word of every squash commit in the repository
with total confidence. An empty prefix therefore reads no subjects at all: nothing downstream
can tell a wrong link from a right one, so "which issue is this about" has to answer nothing
rather than guess. A trailing full stop is trimmed before the check, since a subject that ends
in one is ordinary English and an id that ends in one is not something isu generates. A run of
two or more stops ends an id wherever it appears, because `..` is no part of any key; a single
interior one is left alone, because `ISU-1.2` is a key somebody could have imported and
truncating it would turn a right link into a wrong one.

**A revert is the one commit whose subject means the opposite of what it says**, and the
subject tier skips it — `git revert` quotes the subject it undid verbatim, so the anchor is
present and points backwards, and M7-S3 would record the commit that un-resolved an issue as
one that resolved it. `Reapply "…"` is skipped with it. The trailer tier is deliberately *not*
guarded: git writes the body of a revert itself and does not carry the original trailers over,
so an `Isu-Resolves:` on one was put there by somebody who meant it.

**How a trailer's value is read cost two attempts, and the second is the rule.** Splitting on a
comma alone is isu's own habit mistaken for a convention — git says nothing about what goes
inside a trailer value — so `Isu-Resolves: ISU-7f3akq ISU-39ka2p` named nothing, fell through
to the subject tier, and resolved whatever the forge had put there instead. The first fix
demanded every token look like a key, which broke the ids that do not: `4821` from a GitHub
import and `ISU_7f3akq` are ids, `ValidID` admits them on purpose, and dropping them
reintroduced the same wrong link through another door. So the rule depends on how many things
are in the value: a comma-separated element is judged only by `ValidID`, and an element holding
several words must be all key-shaped, because `the login one` is three legal ids and reading
the value as one token was the only thing filtering prose out. A value folded across indented
lines is read whole, per git's own grammar, rather than losing every claim after the first.
**Branch** `isu/M3-S5-squash`
**Build** nothing new. This story exists to prove the model survives the merge strategy most
teams use.
**Tests first** an end-to-end lifecycle using **only** squash merges: report → triage →
claim → fix → merge → revert, asserting the derived status at every step. Then assert the issue
id is recoverable from the trunk commit that set `resolved` **via the `Isu-Resolves:` trailer**,
and separately that the subject-parsing fallback recovers it from a squash subject that carries
the id, and returns nothing rather than a wrong answer on one that is only a pull request
title.
**Done when** the squash lifecycle test passes without changing M3-S1..S4.

---

# M4 · CLI ✅

**Status** done — all eight stories landed in one pull request rather than eight. That was asked
for explicitly, as it was for M1, M2 and M3, and §0 now allows it outright. Eight commands in one
pull request sits at the top of what §0's one-sitting test tolerates, and it is why that test is
written down: M5's seven checks do not depend on each other and are better off as two or three
pull requests than as one. The milestone boundary rule applies as ever — M5 does not start
without explicit approval.

**One correction, two defects and three limitations came out of this milestone**, and each is
recorded where the decision lives rather than only here: the claim design's compare-and-swap does
not hold between two claimants who share an identity, and section 1 above is corrected; `isu
triage` read the issue at trunk and so failed on precisely the reports it exists for; a
repository whose trunk is called neither main nor master could not resolve anything; and M4-S8's
notes record what the board cannot see across a team, what `fetch_warn_hours` actually measures,
and what the trunk history walk costs per commit.

**M2-S1's grep is amended, narrowly.** Its rule is that internal/gitx is the only place that may
build a *git* command; its proxy was that nothing outside it names `exec.Command`. `isu comment`
opens `$EDITOR`, which is not a git command and cannot be made into one, so one file —
`internal/cli/editor.go`, named rather than matched by a pattern — is exempt from the grep and
asserted directly not to run git.

### M4-S1 · Command scaffold and output contract ✅
**Done** #9, 2026-08-31. The two decisions this story had to make are recorded above: `isu init`
answers section 1's open question, and `--ref` no longer defaults to HEAD — it defaults to what
origin/HEAD says, then main, then master, then HEAD. Half of what this product says is how a
branch differs from trunk, so a trunk that follows you onto your feature branch answers every one
of those questions with "it does not", and leaves `isu resolve` unable to tell that it is about
to write straight onto trunk. **Output is newline-delimited JSON**, one complete value per line,
which is what makes M4-S3's `isu ready --json | head -1` the top of the queue rather than an
opening brace. The gate the story asks for is a table every registered command needs a row in;
beside it, a second test reflects over the payload structs and fails when a field exists in the
code and not in `docs/json.md`, so the contract and its document cannot drift by more than one
commit nobody ran the tests on.
**Branch** `isu/M4-S1-cli-scaffold`
**Build** cobra root, `--repo`, `--ref`, `--json`, `--no-color`, `--fetch`. **Every command
supports `--json`**, because half the users are agents. JSON shape is a stable contract:
document it in `docs/json.md` in this story.
`--fetch` updates remote refs before reading. Every derived status in this product is a
statement about refs, so a board computed from a week-old fetch is a board about last week —
and nothing else in the plan so much as mentions fetching.
**Tests first** golden-file tests for help output; a test asserting every registered command
accepts `--json`; a test asserting JSON output is valid and matches the documented schema; a
test asserting `--fetch` is a no-op against a repo with no remote rather than an error.

**The end-to-end harness ships with the first command, not after the last.** Noted in #8, where
measuring coverage found the honest gap: everything through M3 is tested end to end *through the
stack* — a real git repository, no mocks anywhere, M3-S5 walking a whole lifecycle — but nothing
is tested end to end *through the product*, because there is no product yet. `cmd/isu` is one
line delegating to a `run` that takes its streams as arguments, which is the shape that makes
this cheap; M4-S1 is where that stops being a convenience and becomes the thing every later
story's test is written against. A milestone that builds eight commands and writes its first
end-to-end test at M4-S8 has seven commands nobody ever ran.
**Done when** adding a command without `--json` fails the test suite.

### M4-S2 · `isu board` and `isu show` ✅
**Done** #9, 2026-08-31. Both print the freshness line and warn past `fetch_warn_hours`. **The
`Issue` payload grew the five fields a type requires** — `repro`, `acceptance`, `question`,
`reason`, `resolution` — on every issue rather than only on `isu show`, because M4-S3 wants
`isu ready --json | head -1` to be the whole briefing and what somebody picking work up reads
first is the acceptance criteria or the repro. The body stays one `isu show` away. **The body is
printed verbatim rather than rendered**: glamour is in §0's allowlist for the TUI, where a
renderer earns its place, and it is not needed to print a paragraph. `isu board` renders this
repository, which is the story's own done-when, and a test runs it against this working copy.
**Branch** `isu/M4-S2-board-show`
**Build** the derived board grouped by status, and single-issue detail including attachments,
comments, claim and epic position. Both print a **freshness line** — how old the newest remote
ref is — and warn once it exceeds `fetch_warn_hours`. Two engineers looking at the same repo with
different fetch ages see different contention, and the output should say so rather than let
them argue about it.
**Tests first** golden output for a repo in every status; `--json` round-trips through
`encoding/json` into the documented struct; a repo fetched 3 days ago renders the warning and a
freshly fetched one does not.
**Done when** `isu board` on the isu repo itself renders this plan's milestones.

### M4-S3 · `isu new` and `isu ready` ✅
**Done** #9, 2026-08-31. Three decisions, all forced by "new produces a valid issue" meeting the
schema in section 1. **`new` requires the field its type requires**: a bug with no `repro` is not
a valid issue, so refusing it is the schema surfaced at the command rather than at `isu check`,
and the alternative was writing a placeholder into a required field, which is inventing content
nobody wrote. **`--owner` defaults to git's `user.name`** and asks when git has none, because
`owner` is required and there is exactly one honest guess about who is filing. **`ready` excludes
epics**: an epic has no state of its own and is finished when its children are, so it is never a
thing to pick up — an agent handed one would have nothing to do and no way to say it was done. It
excludes an unreadable issue for the same shape of reason.
**Branch** `isu/M4-S3-new-ready`
**Build** `new` scaffolds a folder, generates an id per M1-S4 and **regenerates it if the token
collides with any id it can see** (trunk plus local refs — this is the story that has the loader
M1-S4 lacks), requires `--title`, stamps `created`, and defaults `priority` to `p2`. It also
takes `--type` (default `bug`) and `--parent`; **`--type epic` omits `state:`**, and is the only
way to create an epic. By default it creates a `report/<ID>` branch and stages it for a pull
request; **`--no-branch`** writes into the working tree instead, which is what a bulk conversion
needs and what M5-S7 uses.
`ready` lists open issues whose `blocked_by` are all terminal — **an epic blocker is terminal
when its rollup is**, since an epic has no `state:` of its own — ordered by priority then age,
`--json` by default for agents.
**Tests first** `new` produces a valid issue and a branch not on trunk (status: awaiting
triage); `new` without a title is refused; two `new` runs in the same second produce different
ids; a seeded collision with an existing id is regenerated rather than returned; `--type epic`
produces an issue with no `state:` that passes M1-S2, and any other type without one fails;
`--no-branch` leaves the repository on the branch it started on; `--parent` naming a non-epic is
refused; `ready` excludes blocked, dropped, claimed and contended issues; `ready` treats an
issue blocked by a fully-resolved epic as ready and one blocked by a half-done epic as not;
`ready` puts a `p0` ahead of an older `p2`.
**Done when** `isu ready --json | head -1` gives an agent everything it needs to start.

### M4-S4 · `isu claim` and `isu unclaim` ✅
**Done** #9, 2026-08-31. **This story found the hole in the claim design, and section 1 is corrected
above**: the stress run put a hundred clones on one issue and thirty of them won, because two
claimants under one identity in the same second write the same commit. The claim commit now
carries an `Isu-Claim:` nonce, the race has one winner, and a deterministic regression test claims
the same issue twice under one identity and asserts the commits differ. Everything else is as
specified: three steps in order, **no checkout at any point** — the commit is built through a
temporary index, so claiming works mid-edit and a lost race leaves nothing behind — the branch
deleted when the push loses, and `unclaim` flipping the state back without deleting anything. The
hundred-run variant lives behind `//go:build stress` and `make stress`, out of the default suite.
**Branch** `isu/M4-S4-claim`
**Build** the three-step claim from section 1, in that order — branch, the state flip committed
with subject `claim <ID>`, push. Claim failure must be fast, quiet and exit non-zero, naming
the branch's holder. There is **no ref-namespace failure left to distinguish and no
`--no-claim` mode to build**: the claim is a branch, so a rejected push is a lost race and
nothing else. `unclaim` flips the state back to `open` and pushes, and never deletes the
branch.
**Tests first** the rejection path **deterministically**: claim from one clone, then claim the
same issue from a second and assert the failure names the holder and exits non-zero. **Assert
the pushed branch is not merely the trunk tip** — a push of the bare tip is a no-op
fast-forward git accepts from both claimants, and the state flip is the whole reason that
cannot happen here, so the test that would have caught the earlier design belongs in this one.
Repeating a network operation a hundred times per CI run buys confidence in the network, not
the code — so the hundred-run stress variant lives behind `//go:build stress` and is out of the
default suite. Also: claiming an already-terminal issue is refused; unclaim leaves the branch
and any work on it in place, and leaves the file byte-identical to what trunk says; **claiming
with the remote detached fails without leaving a local branch behind**.
**Done when** the deterministic rejection test passes and `make stress` exists for the rest.

### M4-S5 · `isu resolve` and `isu drop` ✅
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

### M4-S6 · `isu comment` ✅
**Done** #9, 2026-08-31. **This is the story that amended M2-S1's grep**, as recorded at the head of
this milestone. Beside that, one thing changed a layer up: `isu show` now lists an issue's folder
by walking it rather than through `issue.Load`, which decodes the README on the way past. What is
beside an issue is not a fact about the issue, and a half-written README must not take its
attachments and its comments off the screen — the same choice §2 made in the loader, arriving in
the renderer.
**Branch** `isu/M4-S6-comment`
**Why** `comments/` has been in the data model since section 1 and is rendered by the TUI, but
nothing in the plan ever wrote one. A tracker whose only write path is hand-authoring a file is
not a tracker.
**Build** `isu comment <ID> -m` and `$EDITOR`, appending `comments/<date>-<author>-<nn>.md`
with the sequence chosen by looking at what is already there.
**Tests first** two comments by the same author on the same day produce `-01` and `-02` and
neither overwrites the other; a comment containing `---` at the start of a line does not
corrupt anything downstream; a comment on a nonexistent issue is refused; the issue's
`README.md` is untouched, asserted with `git diff --exit-code`.
**Done when** commenting is one command and the result renders in `isu show`.

### M4-S7 · `isu triage` ✅
**Done** #9, 2026-08-31. **The first version read the issue at trunk, and so failed on precisely the
issues triage exists for** — `awaiting triage` is *defined* as a folder trunk has never seen. It
now reads the issue from wherever it is and writes it where it is going, which also makes
`--push` accept a report onto trunk in one commit. The branch it writes is `triage/<ID>`, chosen
here rather than in this document: it is deliberately outside the `isu/` namespace, because a
triage edit does not flip the state and so is not a claim, and the board says nothing about it
until it merges.
**Branch** `isu/M4-S7-triage`
**Why** Everything the plan can express about an issue other than its state — who owns it, what
blocks it, which epic it belongs to, how urgent it is — had no command. Six reports filed on a
Friday should not wait for Monday's review queue to become answerable.
**Build** `isu triage <ID>` setting `owner`, `parent`, `blocked_by` and `priority`. Defaults to
branch-and-pull-request like everything else. `--push` commits straight to trunk, and is
refused unless `.isu.yml` sets `direct_triage: true` — the field is how a team says out loud
that triage is not a code review.
**Tests first** each field round-trips; `--push` without the config flag is refused with a
message naming the flag; `--push` with it produces exactly one trunk commit; `parent:` naming a
non-epic is refused at the command rather than left for CI; unknown priority is refused; the
body and unknown keys survive byte-for-byte.
**Done when** a report can be triaged without opening a pull request, if and only if the
repository has said that is allowed.

### M4-S8 · First contact with a real repository ✅
**Done** #9, 2026-08-31. Three full clones — openssl/openssl for its 11 submodules and 26 years,
facebook/react for its 967 remote branches and its squash-merge habit, golang/go for the biggest
tree. All three load; `docs/field-notes.md` has the timings and the ref counts. One defect found
and fixed with a regression test, and three limitations written down, of which one is serious
enough to belong here: **the board reads `refs/heads/` and so cannot see anybody else's claims.**
A claim reaches other people as `refs/remotes/origin/isu/<ID>`, which the board does not read, so
contention across a team — the thing claiming exists to prevent — is invisible. The freshness
line is the evidence that this is not what was intended, since fetch age cannot affect contention
unless remote refs feed it. Reading `refs/remotes/` costs 3.5 ms a ref, measured. It is not fixed
here because the ref pattern is M2's and the self-contention it would cause — a claimant's own
board reading their local branch and its remote-tracking twin as two claims — is M3's. **It wants
a story of its own, and it should get one before M5-S5 reports contention to anybody.**
**Branch** `isu/M4-S8-field-notes`
**Why** Everything so far has run against fixtures written by the same person who wrote the
assumptions. This is the first story where the world gets a vote, and it is deliberately
before the TUI and the importers are built on top of those assumptions.
**Build** run the CLI's read paths against three real repositories of different shapes: a
large monorepo, one with submodules, and one with a decade of history and heavy squash-merge
use. Read only — nothing is written and nothing is imported. Record what happened in
`docs/field-notes.md`, including the timings and the ref counts.
**Tests first** every defect found becomes a regression test with a fixture reproducing it,
written before the fix.
**Done when** all three load, and every surprise is either fixed or written down as a known
limitation.

---

# M5 · Checks ✅

**Status** done — all seven stories landed in one pull request rather than the two or three
this document recommended at the head of M4. That was asked for explicitly, and §0 allows it;
it is worth recording that the recommendation was right about the shape and wrong about the
seam. The seven checks really are independent, but they are all one Input away from each
other, and the two loaders that build that Input — what a branch proposes over trunk, and what
lives beside each issue — are M5-S1's whether they are used by one check or by four.

**The command this milestone needed did not exist yet.** `isu check` is not in M4's list of
eight, because until there was a check engine there was nothing for it to run; it arrives here,
with a row in the JSON contract, a golden help file that lists the rules themselves, and the
same `--json` obligation as everything else.

**Two flags are decisions this document did not make, and both are the pre-commit hook's
fault.** `--scope tree|branch|all` exists because the hook cannot ask the branch questions: at
pre-commit time the change being committed is not a commit yet, so those rules would be
answered from the commits already on the branch — and on a claiming branch that answer is "you
resolved an issue and wrote no code", which would block the very commit that writes the code.
`--worktree` exists because the tree questions have the same problem in a worse form, and
M5-S6 records what testing it found: a hook reading refs refuses the commit that fixes what it
is complaining about, which is a deadlock and not a gate.

**One defect and one carve-out came out of the rules themselves.** The duplicate-id check
reported one issue claimed on two branches as two issues wearing one id, found by M5-S5's first
two-branch fixture; and the evidence check does not demand code from an issue a branch
*created* in a terminal state, because that is what an import is and M7 would be
unimplementable otherwise. Both are recorded at the story that owns them.

**M4-S8's limitation is now load-bearing and still open.** The board reads `refs/heads/`, so a
claim that reached this clone as `refs/remotes/origin/isu/<ID>` is invisible — and M4-S8 asked
for a story of its own "before M5-S5 reports contention to anybody". That story has not been
written and this milestone did not invent it: instead every contention and staleness warning
says which refs it read, so a local run that disagrees with CI is self-explaining rather than
confidently wrong. **The story is still wanted, and the proposal is in the pull request.**

**The branch is not the one §0 names.** A branch carrying several stories is named for the
range and its subject — `isu/M5-S1-S7-checks` — and this work was done on
`claude/next-milestone-7r5wtq`, which the session that produced it was told to use. The story
ids are in every commit subject, where the rest of §0 puts them.

**One thing this milestone did not do to its own history.** The `--worktree` correction was
committed together with M5-S7's conversion rather than in its own pair of commits; the split
could not be made after the fact in the environment the work ran in. The code and its tests are
in that commit, and this line is here so a reviewer looking for them knows where they went.

### M5-S1 · The check engine ✅
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

### M5-S2 · Structural checks ✅
**Done** #11, 2026-09-01. Six rules, and half of them are the other half of a sentence M1-S2
could only start: `Validate` takes one issue and nothing else, so `parent:` is checked for shape
there and for what it names here. **Every copy is checked and not only the one derivation
renders** — an issue that is fine at trunk and broken on the branch proposing it is a broken
issue, and the finding names the branch so nobody goes looking at trunk for it. **The
duplicate-id rule is about ids trunk has never seen**, and has to be: an id on trunk and on a
branch is one issue somebody edited, which is the model working. Two branches carrying an id
trunk has never seen, with different titles or creation dates, are two issues wearing one id.
Its first version got that wrong in a way only a two-branch fixture found, and M5-S5's
contention fixture is what found it: it meant to skip the ids trunk already carries and instead
appended the branch copies to the empty entry it had just made for them, so one issue claimed
on two branches read as a corruption. The rule now has two fixtures asserting it does *not*
fire — one issue edited on two branches, and a report triaged on a second branch — because a
rule that fires on the ordinary case is worse than no rule at all.
**Branch** `isu/M5-S2-structural-checks`
**Build** schema validity, id matches folder, duplicate ids, `parent` and `blocked_by` exist,
**`parent` names an issue of `type: epic`**, parent cycles, self-parent, dependency cycles,
**an epic declaring its own `state:`**, **an epic with no children**, and the attachment size
cap from `attachment_max_bytes`.
**Tests first** one repo fixture per violation, and one clean fixture asserting zero findings.
The duplicate-id fixture matters more than it used to: it is now the only thing standing
between two clones that generated the same token at the same moment and a corrupt tree.
**Done when** every rule in section 1 that can be checked without a diff is checked.

### M5-S3 · Evidence checks ✅
**Done** #11, 2026-09-01. **One carve-out, and it is a decision this document did not make.**
Resolving means the branch found the issue open here and left it terminal. An issue the branch
*created* in a terminal state is not a resolution: it was never open in this repository, which
is exactly what an import is — M7-S2 writes thousands of issues Jira closed years ago, on a
branch that changes nothing outside `issues/` because there is nothing else to change. A rule
that demanded code for those would catch nobody and would make the importer unimplementable.
The `reason` and `resolution` a drop requires stay the schema's to report: `state: dropped`
without them does not satisfy `Validate`, and two rules reporting one line would give a reader
two things to fix that are one thing. A drop that also changes code warns and exits 0, as
specified.
**Branch** `isu/M5-S3-evidence-checks`
**Build** the type table's resolution rules: resolving requires a change outside `issues/`;
a spike requires an artifact in its own folder; a drop requires `reason` and `resolution`.
A drop that also changes code is a **warn, not a fail** — closing a duplicate in the same pull
request as the fix is a normal thing to do, and refusing it just teaches people to split the
work into two reviews.
Only applies to issues the branch actually changed.
**Tests first** resolved-with-no-code fails; resolved-with-code passes; **a branch carrying
nothing but a claim fails, and that is the check doing its job** — claiming writes
`state: resolved` and touches nothing else, so this check is exactly what separates a claim
from a resolution and the board's `in progress` from `done`; spike with only `README.md` fails;
spike with `decision.md` passes; drop without `resolution` fails; drop with code changes warns
and exits 0.
**Done when** an agent cannot mark work done without doing it.

### M5-S4 · Owner immutability ✅
**Done** #11, 2026-09-01. It reads the commit author rather than the diff, because the
asymmetry is the whole rule: the same one-line change is fine from a person and is not fine from
the thing they are supervising. An author is matched against `agents:` by name as written and by
address without regard to case — two display names are two decisions somebody made, where two
spellings of an address are one mailbox. A branch is not one author, so the finding lands on the
commit that did it and a person's commits beside it neither excuse it nor are blamed for it.
**Branch** `isu/M5-S4-owner-immutability`
**Build** if the commits on this branch change `owner:` and their author is in the configured
`agents:` list in `.isu.yml`, that is a `fail`. Humans may change it freely.
**Tests first** agent-authored owner change fails; human-authored passes; agent changing
`state` but not `owner` passes; a mixed-authorship branch fails on the agent's commit.
**Done when** accountability cannot be reassigned by anything that isn't a person.

### M5-S5 · Contention and staleness reporting ✅
**Done** #11, 2026-09-01. Both are warnings and neither fails a pull request: one refused for
contention is one refused for somebody else's branch. **Every finding carries a clause naming
what it was read from**, which is this story's own requirement and also the honest answer to
M4-S8's open limitation — a repository with no remote is told these are local branches only, a
stale ref set is told how old it is and to run `--fetch`, and a repository with a remote is told
that a claim somebody else pushed and never merged is not in the answer at all. That last clause
is a stopgap for the story M4-S8 asked for and is not a substitute for it.
**Branch** `isu/M5-S5-contention`
**Build** warn when another branch claims the same issue, naming the branch and holder; warn on
claims older than `stale_days`. Both warnings are statements about refs, so both are
only as true as the last fetch: run `--fetch` in CI, and include the fetch age in the warning
so a local run that disagrees with CI is self-explaining.
**Tests first** two branches claiming the same issue produces one warning naming the other
branch; stale claim produces a warning with the age in days; a stale local ref set produces a
warning that says so rather than reporting confident nonsense.
**Done when** both surface at pull-request time rather than at merge.

### M5-S6 · Hooks and CI templates ✅
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

### M5-S7 · Dogfooding switch ✅
**Done** #11, 2026-09-01. Twenty-nine issues: five epics, one per remaining milestone, and
twenty-four stories, one per remaining story heading — all through `isu new --no-branch`, with
`--parent` naming the milestone and `--blocked-by` naming the story before it, so that the first
story of a milestone waits on the milestone before it and an epic blocker is terminal when its
rollup is. Each story's `acceptance` is its own **Done when** sentence, which is what that line
has always been. **This pull request's own seven stories are flipped to `resolved` by `isu
resolve`**, one commit and one `Isu-Resolves:` trailer each, which is the mechanism this story
turns on for everything after it.

**The test reads the working tree where this document says trunk, deliberately.** An assertion
about trunk is one that cannot fail on the pull request that breaks it — trunk has not merged it
yet — so it would be green for the whole of the review and red immediately afterwards, which is
the one moment nobody is looking. Beside the four assertions this story asks for, it holds
PLAN.md and `issues/` to each other in both directions: every story heading from M5 on has an
issue with that title, and every issue is one of those headings. Two documents saying the same
thing drift the moment nobody is checking.

`make dogfood` is the gate, and CI runs it against the base branch it fetches by name. The
binary is the one just built rather than a release: a pipeline that checked this repository with
last month's isu would pass the pull request that broke the checks.
**Branch** `isu/M5-S7-dogfood`
**Why** Last in this milestone on purpose. It needs `isu new` from M4-S3 to create the issues
and the whole check suite to keep them honest; converting any earlier means hand-maintaining
issue files with nothing verifying them.
**Build** convert every remaining story in this plan into an issue folder using
**`isu new --no-branch`**, so the whole conversion lands on this one branch and reaches trunk in
this one pull request — the default `report/<ID>` branch per issue would scatter fifty issues
across fifty branches that the test below, which reads trunk, could never see. Milestones become
`--type epic`; every story is created with `--parent` naming its milestone, and `blocked_by`
encodes the order. Turn `isu check` on for this repository's own pipeline.
**Tests first** a test loading `issues/` from trunk asserting every issue validates, the
dependency graph is acyclic, every epic has at least one child, and every non-epic names a
`parent` that is an epic. That last one is an assertion about **this repository's tree**, which
the conversion controls — `parent` stays optional in the schema, because a repository with no
epics at all is a perfectly good repository.
**Done when** isu tracks its own construction and its own CI enforces it. From here on every
pull request also flips the issue file of each story it carries to `resolved` — and M5-S3 turns
that into a failing check if the pull request contains nothing else.

---

# M6 · TUI ✅

**Status** done — all five stories landed in one pull request. §0's grouping rule allows it: they
are neighbours in this document, they belong to one milestone, and they share one subject a
reviewer can hold at once — the branch would have been `isu/M6-S1-S5-tui`, and this work was done
on `claude/next-milestone-gzyx59`, which the session that produced it was told to use. The story
ids are in every commit subject, where the rest of §0 puts them.

**`bubbles` is in the allowlist and is not used.** The list, the filter and the detail pane's
scroll are a handful of statements each over state this package already holds, and a component
that brings its own key map and its own styles would have been more to reconcile than to write —
the filter alone would have had to have its blinking cursor turned off to keep a golden frame
stable. bubbletea, lipgloss and glamour are all used, and teatest is used for what M6-S1 asks it
for. Removing a dependency the allowlist permits is not adding one, and go.mod says so.

**Two keys the map in M6-S1 does not name.** `h`/`←` folds an epic and `l`/`→` unfolds it, because
M6-S3's "navigating across a collapsed epic" requires a way to collapse one and this document
names none; and `esc` clears a filter, closes the filter line, and gives the keys back from the
detail pane. `q` quits from everywhere, including the pane `enter` opened — a key that quits from
one pane and does something else from another is the one thing a person has to keep in their head.

**`isu ui --json` prints `isu board`'s payload**, and that is a decision this document does not
make. Every command supports `--json` and `contract_test.go` enforces it, but an interface is not
a thing an agent can read; refusing to answer would leave one guessing, and inventing a second
shape would be a contract nobody asked for. What the interface would open on is the board.

### M6-S1 · Shell, layout and key map ✅
**Done** #12, 2026-09-02. `internal/ui` is a pure function of what it was handed, the way
`internal/model` is a pure function of what `internal/repo` loaded: the whole repository arrives
as an `Input` — the groups `isu board` renders, the derived board behind them, the freshness
sentence and the moment — and a frame is that plus which keys have been pressed. That layering is
what makes M6-S5's structural test easy to pass rather than something to arrange: a package that
cannot load anything cannot call git by accident.

Six lines of chrome whatever the terminal is, and **the message line is kept even when there is
nothing to say** — a footer that grows a line when an action reports moves the list up by one
under somebody's cursor, and the moment after `c` is the moment they are looking hardest. Every
line of a frame is trimmed on the right, which is not cosmetic: a pane padded to its width leaves
a run of spaces that is invisible on a terminal and very loud in a golden file.

**`-update` is not this package's flag.** The golden-file helper teatest brings registers one of
that name, and two flags of one name is a panic at init rather than a warning, so the golden
helper here reads it instead of declaring it.
**Branch** `isu/M6-S1-tui-shell`
**Build** bubbletea program: header with status counts, list pane, detail pane, footer key
hints. Resize-aware down to 80×24. Keys: `enter` open, `c` claim, `n` new, `r` ready queue,
`g` go to branch, `/` filter, `q` quit.
**Tests first** `teatest` golden frames at 80×24 and 140×40; a resize sequence; `q` quits
cleanly and restores the terminal.
**Done when** golden frames are stable across runs.

### M6-S2 · List and grouping ✅
**Done** #12, 2026-09-02. The grouping is one function used twice rather than two orderings
that agree: `view.itemGroups` buckets by derived status in the precedence order of §1's table,
`isu board` renders it as JSON and `isu ui` hands the same slice to the interface. That is what
"matches exactly" can mean and keep meaning, and a shared fixture in `internal/cli` asserts it
against a real repository.

**Within a group the order is the board's, with one thing done to it**: a child whose epic is in
the same group is drawn immediately after that epic, one level in. Which group an issue is in is
untouched, so a child whose epic has finished stands at the top of its own group rather than under
an epic three groups away — the alternative, nesting across groups, would have meant a list whose
statuses no longer add up to the counts above them. A folded epic says how many rows it is holding
back beside its title rather than beside its id, because a marker glued to an id is a marker
somebody copies with it.

Every issue is emitted once. A parent cycle is two epics that are each other's ancestors, which is
`isu check`'s to report and this arrangement's to survive: the walk marks what it drew and a sweep
at the end draws whatever it could not reach.
**Branch** `isu/M6-S2-tui-list`
**Build** the issue list with epics as parents and their children indented beneath, status
colouring, and the counts in the header staying consistent with the list beneath them.
**Tests first** golden frames for a list with nested epics; an epic with forty children; a
list of zero issues rendering the empty state rather than a blank pane.
**Done when** the grouping matches `isu board` exactly, asserted by a shared fixture.

### M6-S3 · Filter and navigation ✅
**Done** #12, 2026-09-02. Every printable key goes into the needle while the filter line is
open, which makes the whole command map unreachable there — a `q` that quit half way through
typing "queue" would make the filter unusable — and the key hints change with it, because offering
`c claim` on a line that cannot claim is offering something that does not happen. The arrows still
move.

**The cursor remembers what somebody chose rather than where it happens to be.** A filter that
hides the selected issue moves the cursor; clearing it puts them back, because the thing they
picked is still what they picked. Only a deliberate move changes that, which is the difference
between the two halves of this story's own sentence.

**The frame budget was missed on the first draft and the fix was one line of design.** Lowercasing
five fields per issue on every keystroke is twenty-five thousand allocations a keypress on a
five-thousand-issue board: 5–8 ms against a 16 ms frame. Each issue's searchable text is
lowercased once at startup instead — it cannot change underneath the index, because nothing in
this package loads anything — and the same keystrokes now cost **2.6 ms**.
**Branch** `isu/M6-S3-tui-filter`
**Build** incremental filter across id, title, type, status and owner; vim and arrow keys;
selection preserved across filter changes where the selected issue still matches.
**Tests first** narrowing then clearing restores the previous selection; navigating across a
collapsed epic; filtering to zero results and back; a filter string containing regex
metacharacters is treated literally.
**Done when** filtering a 5,000-issue fixture stays inside one frame budget.

### M6-S4 · Detail pane ✅
**Done** #12, 2026-09-02. "Can I start this?" is a question with four parts — what is it, has
anybody got it, what is it waiting on, and what does done look like — and the pane answers each:
the type's own required field, both claimants on a contended issue, every blocker with its own
status, and an epic's children with theirs. A child is told where in its epic it sits, because
being one of forty is a different proposition from being the last of three.

The body is rendered rather than printed, which is where glamour earns its place in the allowlist
and what `isu show`'s "v1.0.0 does not render markdown" was waiting for. A pane too narrow to wrap
into gets the body as it was written, which is the answer `isu show` gives anyway.

**What lives beside an issue is loaded, so it arrives through an action and is asked for once** —
the answer costs a git process and the cursor walks over the same issue every time somebody
scrolls past it. A folder that could not be read is kept as the failure it was: never failing
silently is M6-S5's rule and it applies to the read as much as to the writes.

`enter` gives the keys to the pane so it can be scrolled and `esc` gives them back. The wrap this
story needed also fixed one M6-S1 had shipped: folding rebuilt a line out of its words, which
collapsed the two spaces holding a key away from its value, so the first fold in a pane unaligned
every column in it.
**Branch** `isu/M6-S4-tui-detail`
**Build** body rendered with `glamour`, title, priority, claim line, acceptance/repro,
attachments, comment list, epic position, and `blocked_by` showing each blocker's own status.
**Tests first** golden frames for each type; an issue with twenty attachments scrolls; a
contended issue shows both claimants.
**Done when** the detail pane answers "can I start this?" without leaving the TUI.

### M6-S5 · Actions from the TUI ✅
**Done** #12, 2026-09-02. `claimIssue` and `report` are `isu claim` and `isu new` with the
command taken off the front, so the interface calls the function rather than something that agrees
with it. `checkout` is new and is deliberately not a claim: `g` on an issue nobody has claimed
from this clone is a sentence, because making a claim as a side effect of navigating to something
would claim work for whoever pressed a key by mistake.

`n` opens the editor on **a whole issue file rather than a form**. What comes back is parsed by
`internal/issue` and validated by the schema every other issue is held to, so there is no second
format to keep in step — and the id is allocated after the edit, because it is a hash of the
fields the editor is for. The terminal goes with it: `tea.Exec` releases the screen for as long as
the editor runs, and this package hands over a `Run()` rather than a process, which is how it
keeps the structural rule below.

**The proof M6-S5 asks for is stronger than the grep `internal/gitx` already runs.** That one says
nothing outside gitx builds a git command; this one says `internal/ui` does not import the git
binary or either of the two packages that reach it. A renderer that could run a git process would
run one per frame.

**Two things this milestone had to learn about terminals, and both were defects.** A burst of
printable characters arrives as one message carrying several runes — that is how a paste and a
fast typist both look — and every rune but the first was being dropped, so the interface lost keys
under exactly the condition somebody is going fast. And `q` pressed while an action is in flight
is now remembered rather than obeyed: leaving in the middle of a push would abandon the one
operation in this product that has to be atomic.

**teatest does not survive `tea.Exec`, and that is worth writing down.** Measured at about one run
in two, the program released the terminal and never repainted — and every run once the test waited
for a frame before typing. The model was right each time; what was wrong was synchronising on an
intermediate byte stream. The action tests run the same program over an ordinary pair of buffers,
which is what `isu ui` itself runs over, and wait on the fake rather than on the screen.
**Branch** `isu/M6-S5-tui-actions`
**Build** `c` claims through the same code path as `isu claim`; `n` opens an editor for a new
issue; `g` checks out the claiming branch. Every action reports its result inline and never
fails silently.
**Tests first** claiming an already-claimed issue shows the holder; the editor is injected and
tested with a fake; `g` on an unclaimed issue is a no-op with a message.
**Done when** the TUI shares command implementations with the CLI, proven by a test that fails
if the TUI package calls git directly.

---

# M7 · Importers

**The importer is GitHub Issues, and it used to be Jira.** The swap is less a change of target
than a change of what the source can be asked. Somebody adopting isu is already in a git
repository, that repository is overwhelmingly on GitHub, and the issues they want out are
therefore sitting beside the code they are migrating — no export request, no admin, no licence.
GitHub also answers for free two questions Jira answers through an integration somebody had to
install: **which pull request closed this issue**, and **what is this issue blocked by**. And it
can be read without credentials, which is what lets these stories be tested against real public
repositories rather than against a fixture nobody can check. Jira joins Linear on the
out-of-scope list as a deferral, and M7-S1's source interface is the seam it comes back through.

**Four things GitHub Issues can now do did not exist when this milestone was first written**,
and each is read where present rather than assumed: **issue types** (organisation-level, at most
twenty-five, `type` on the issue itself, defaulting to Task/Bug/Feature); **sub-issues** (100
children per parent, eight levels of nesting, and they may cross repositories); **issue
dependencies** — `blocked_by` and `blocking`, August 2025 — which is isu's `blocked_by` under
its own name; and **issue fields**, structured custom metadata that reached general availability
in July 2026 and is precisely the two hundred custom fields M7-S1 refuses to let into
frontmatter. A repository with none of them still imports, and the dry run says which it found.

**Ids are the one place the data model had to be amended, and section 1 carries the amendment.**
A Jira key is already a legal id; a GitHub key is `#1234`, which is repo-scoped, shares its
sequence with pull requests, and is not a folder name. `<PREFIX>-1234` it is.

### M7-S1 · Import framework and dry run
**Branch** `isu/M7-S1-import-framework`
**Build** `internal/importer`: a source interface, field mapping to the isu schema, an
evidence-tier recorder, and a `--dry-run` report showing counts, coverage and samples without
writing anything. **A source key becomes the id verbatim where it is already a legal one, and
is formed from it where it is not** — `PROJ-1234` stays `PROJ-1234`, GitHub's `#1234` becomes
`<PREFIX>-1234`, and either way the thing your team has been writing in commit messages for
years is still legible in the id. Section 1 governs the shape; this story owns the rule that
the mapping is total and reversible. Unmapped source fields go to `source.yml` in the issue
folder, never into frontmatter — a mature tracker has two hundred custom fields and they must
not poison the schema.
**Tests first** dry run writes no files; an id maps back to the source key it was formed from,
for a key that was legal and for one that was not; a source with two hundred custom fields
produces clean frontmatter and a complete `source.yml`.
**Done when** `--dry-run` is the default and writing requires `--write`.

### M7-S2 · Safe writes
**Branch** `isu/M7-S2-safe-writes`
**Why** An importer writes attacker-influenced data into your repository. Ticket titles,
comment bodies, author display names and custom field values all originate outside your
control, and on a public repository "outside your control" means anyone with a browser.
v1.0.0 never serves that data over HTTP — the web UI is out of scope, and `glamour` renders to
a terminal — so **writing it to disk is the entire attack surface, and it gets its own story.**
**Build** one guarded write path that every importer must use. Filenames sanitised and
constrained to the issue's own folder — reject `..`, absolute paths, symlinks, control
characters, reserved Windows names, and names over 255 bytes. Per-file and per-issue size
caps. API tokens read from the environment or a credential helper, never from a flag, never
written to disk, and redacted from every log line and error string.
**The decompressed-size limit and the content-type sniffing are deferred along with the
downloads that needed them.** v1.0.0's one importer records attachment links and fetches
nothing (M7-S5), so a zip-bomb guard here would be a defence with no traffic on it and a 99%
coverage floor to answer to. They return with the first importer that fetches a file, and the
guard they belong behind is this one.
**Tests first** one payload per attack: `../../etc/passwd`, an absolute path, a symlink, a
4,000-character filename, a filename containing a newline, **a comment author whose display
name is a path** — the comment file is named for the author, so this is the live one — an issue
body over the per-issue cap, and a token interpolated into an error message.
**Done when** every importer writes exclusively through this path, asserted by a grep test in
the same style as M2-S1.

### M7-S3 · Resolving-commit recovery
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

### M7-S4 · GitHub: issues, types, milestones and state
**Branch** `isu/M7-S4-github-core`
**Build** read issues from the GitHub API, or from a saved `gh issue list --json` dump —
`--fetch-only` writes that dump, the tests read one, and an import is therefore reproducible
without a network and reviewable as a diff. **Pull requests are not issues.** The REST list
returns both and they are told apart by the `pull_request` key; dropping them is the first
thing this importer does and the classic bug in every one that skips it.

- **Type.** The organisation's issue type where the repository has one, a configured label map
  where it does not, `chore` where neither answers. GitHub's own defaults map Bug to `bug`,
  Feature to `story` and Task to `chore`; every other type and every label is the map's
  business, and the dry run lists what it could not place.
- **Epics are milestones.** A milestone is a named container whose progress is a fold over the
  issues in it, which is exactly what an isu epic is, and an issue belongs to at most one — so
  it lands in the single `parent` field with no collapse to design. The epic is written
  **without `state:`** like every epic, the milestone's description becomes its body, and its
  own open/closed state and due date go to `source.yml`, because here the fold is what decides.
  Its id is `<PREFIX>-M<number>`: milestones are numbered from their own sequence, so `ISU-M3`
  and `ISU-3` are different issues and must not collide. **A milestone with no imported children
  is not written at all** — M5-S2 makes an empty epic a check failure, and `--state open` empties
  every milestone whose issues are all closed.
- **Sub-issues do not become `parent`.** The field is spent on the milestone, isu has one, and a
  sub-issue tree is eight levels deep where an epic is one. The hierarchy is written whole to
  `source.yml` and its depth and size are reported by the dry run. This is the largest thing
  v1.0.0 knowingly declines to model, it is on the out-of-scope list under its own heading, and
  recording it losslessly is what makes modelling it later a story rather than a re-import.
- **State.** `open` becomes `open`. `closed` with `state_reason: completed`, **or with no
  `state_reason` at all**, becomes `resolved` — the field only exists since 2022 and everything
  older is an ordinary closed issue rather than an unknown. `not_planned` becomes `dropped` with
  `resolution: wontfix`, `duplicate` becomes `dropped` with `resolution: duplicate`, and
  `reopened` becomes `open`, because isu derives reopening from trunk history and will not read
  it from a field it would then have to keep in sync.
- **`blocked_by` is `blocked_by`.** GitHub's dependencies say what isu's field says. A blocker
  outside the import goes to `source.yml` rather than being written as an id that resolves to
  nothing, which M5-S2 would fail on.
- **Owner.** The first assignee; failing that `--owner`; failing that the import refuses and the
  dry run says how many issues have neither. The author is recorded in `source.yml` and is
  deliberately not the owner: the person who filed a bug is usually not the person answerable
  for it, and M5-S4 makes `owner` expensive to correct afterwards.
- **The type's required field.** `repro`, `acceptance` and `question` are required by the
  schema, GitHub supplies none of them, and a naive import would emit thousands of files that
  fail `Validate()` on the first `isu check`. Each is written as a provenance line —
  `repro: imported from owner/repo#1234; see the body` — and the dry run counts them so nobody
  mistakes archaeology for content. The rejected alternative was to call everything a `chore`,
  which validates trivially and throws away the bug/story distinction across the entire history
  in one move.
- **Rate limits are handled, not hoped for.** Five thousand points an hour, a hundred concurrent
  requests, and a cap per minute besides. A 5,000-issue import fits comfortably inside that and
  only if it batches; on a 403 or a 429 the importer waits out `retry-after`, or
  `x-ratelimit-reset` when there is no `retry-after`, then backs off, and the dry run reports
  what the budget cost.

**Tests first** a fixture dump covering issue types and a repository with none, milestones
including an empty one, a four-level sub-issue tree, every `state_reason` including its absence,
dependencies pointing inside and outside the import, and unassigned issues. Assert pull requests
in the list are not imported; assert imported milestones carry `type: epic` and no `state:`;
assert every dropped issue has a `resolution` from the enum and a `reason`; assert an issue whose
milestone was skipped emits no `parent`; assert an out-of-import blocker leaves `blocked_by`
absent rather than dangling; **assert the whole import makes no per-issue request for anything
the list already carried** — a process count in the spirit of M2-S5, against a transport that
counts.
**Done when** the imported tree passes `isu check` with zero failures, and 5,000 issues are read
in pages of a hundred with no request per issue.

### M7-S5 · GitHub: comments, fields and closing pull requests
**Branch** `isu/M7-S5-github-content`
**Build** comments become files under `comments/`, named by date, author and sequence per
section 1 — an active issue routinely has three comments from the same person on the same day,
and without the sequence the importer would silently keep only the last. **Issue field values
join everything else unmapped in `source.yml`**: they are read per issue from
`/repos/{owner}/{repo}/issues/{number}/issue-field-values`, they are the case M7-S1 wrote its
rule for, and a field an organisation invents next year must not need a schema bump here.
**Attachments are recorded rather than downloaded, and that is a finding rather than a
preference.** A GitHub attachment is a `github.com/user-attachments/assets/…` link inside the
markdown. On a private repository it cannot be fetched with a personal access token or a GitHub
App token at all: the asset wants a browser session, and the only programmatic route is to
re-request the issue with `Accept: application/vnd.github.full+json` and race a JWT out of
`body_html` before it expires minutes later. An importer whose completeness depends on winning
that race is one that half-works on exactly the repositories people most want migrated. So
v1.0.0 rewrites nothing: the links stay in the body byte for byte, they are listed in
`source.yml`, and the dry run says how many there are and that resolving them still needs
github.com. This is why M7-S2 defers its decompression limit, and why this story is no longer
the largest in this milestone.
**Closing pull requests are the strongest evidence tier there is, and here they are free.**
GitHub already stores which pull request closed an issue — `closedByPullRequestsReferences`,
and the `closed` timeline event's `commit_id` where a commit closed one directly — and both
arrive with the issue rather than through an integration somebody had to install. They outrank
every tier of M7-S3's scan, which is what that recorder was built to arbitrate.
**Tests first** against a recorded transcript, never the live API: a comment containing
frontmatter delimiters does not corrupt the issue file; three comments by one author on one day
produce three files rather than one; forty custom field values produce clean frontmatter and a
complete `source.yml`; an attachment link survives the body byte for byte and is listed;
`closedByPullRequestsReferences` outranks the M7-S3 scan when both have a link for the same
issue.
**Done when** a realistic export imports completely and idempotently — running it twice
produces a zero-length diff.

---

# M8 · Public website ✅

**Status** done — all three stories landed in one pull request. §0's grouping rule allows it:
they are neighbours here, they belong to one milestone, and they share one subject — the branch
would have been `isu/M8-S1-S3-website`, and this work was done on `claude/milestone-8-fafh5y`,
which the session that produced it was told to use. The story ids are in every commit subject.

**This milestone was taken before M7, and M7 is written rather than absent.** That was the
instruction and it is recorded here rather than smoothed over: `issues/ISU-he1wf1` is
`blocked_by` the M7 epic and was resolved with that blocker open. M7 exists as #15, open against
the same base as this milestone and not merged, so trunk has no importer while this lands and
`docs/importing.md` says so in its first paragraph. Nothing on the site depended on the importer
existing. What the ordering costs is written under M8-S3: one page of documentation that goes
stale the moment #15 merges, and a conflict in this file when it does.

The site is designed and built from scratch in this milestone. There is no approved comp to
port — treat M8-S1 as real design work with a written brief, not as implementation.

The governing constraint: **every sample shown on the site is generated by running this
repo's binary during the build.** No hand-written terminal HTML, no invented output. If the
product changes and the site doesn't, the build fails.

Three stories, not seven. The site sells v1.0.0; it does not gate it, and every week spent here
is a week the thing it advertises is not shipping.

**The samples run in process rather than through the built binary, and that is the same
decision M2-S1 made.** A generator that shelled out to `./isu` would be a second place in
`internal/` that constructs a command, which `TestNothingOutsideGitxExecutesGit` exists to
prevent, and it would be a generator that could be pointed at a stale binary. `cli.Run` takes
its streams, its directory, its clock and its environment as arguments — M4-S1 built it that
way so the tests would be end to end through the product — so `internal/site` calls the same
function `cmd/isu` calls, with a fixed clock. "This repo's binary" and "this repo's code" are
the same thing said twice, and only one of them can go stale.

**`make site` is `go test -update`, not a generator of its own.** The site is a golden file like
every other artifact here: `make site` writes `web/site/` and `make test` fails when the
committed site and the regenerated one differ. A separate `cmd/` would have added a `main`
nobody covers against a floor with nine statements of headroom, and running the package's tests
is what executes the docs' console blocks before anything is published rather than after.

### M8-S1 · Content plan and information architecture ✅
**Done** #21, 2026-09-09. `web/CONTENT.md` is not a brief the page was built *from* — it is the
page. `internal/site/content.go` parses the section sequence, the claim, the copy and the sample
out of it, and the template carries structure and not one word, because a template with a
sentence in it is a second place the site's copy lives.

**The executable-documentation harness is this story's, and M8-S3 reuses it.** A ```console
fence is run; a `$ ` prompt anywhere else — loose in the prose, or in an `sh` fence — fails the
extraction, because a transcript nothing runs is exactly the thing that rots. A command must
begin with `isu`, so a document cannot ask the harness to run something else.

**The build found one defect in `isu init`, and it is not fixed here.** `reportWrite` falls back
to the path list for its headline when there is no issue id, and then prints the same list
underneath — so `isu init` names its three files twice. It is on the site, in
`docs/getting-started.md`, exactly as the command prints it. Fixing it changes M4's output and
its golden files, which is M9-S3's business or a story of its own, not a website's.
**Branch** `isu/M8-S1-content-plan`
**Why** The page has one job: an engineer decides in thirty seconds whether this is a toy.
Decide what must be proved, and in what order, before anything is designed.
**Build** `web/CONTENT.md` — the section sequence, the single claim each section makes, the
artifact that proves it, and the finished copy. Plus the docs tree and the navigation. Then the
design decisions, in the same file and kept short: four to six named colours with hex, the
typefaces and their roles, the type scale, and the one signature element the site will be
remembered by. No markup in this story.
**Tests first** every command and code sample in `CONTENT.md` executes against a scratch repo
and produces the output the document claims.
**Done when** someone who has never seen the project can read `CONTENT.md` and say in one
sentence what isu does and who it is for.

### M8-S2 · Landing page ✅
**Done** #21, 2026-09-09. Seven sections, five of them a claim above a card containing bytes isu
wrote. The card is the signature element: a header bar with the command and a body with the
output, and nothing in between for a designer to embellish.

**"Fonts self-hosted and subset" was answered by shipping no font at all**, and that is the one
place this story's brief was met differently from how it was written. The strongest available
form of "no third-party font CDN" is to have nothing to fetch: prose is set in the reader's own
UI face and terminal output in their own monospace, so there is no subsetting step, no swap on
first paint, and the third-party-request gate passes because there is nothing that could fail
it. If a brand face is wanted later it is a stylesheet change and a file, not a redesign.

**The contrast gate earned its place on the day it was written.** `--signal` on `--terminal` is
3.18:1 and looks perfectly fine; the terminal card's command line uses `--glow` at 9.09:1
because a computed number said so and an eye did not.

**The page was read back three times by a hostile reader, and each pass found something the
gates could not.** They are recorded here because the pattern is the point: every round turned
up at least one place where the site *said* something that was not mechanically true, on a site
whose whole argument is that it never does.

- Round one: section 5 said the out-of-scope page "is linked from here" and carried no anchor;
  a `sh` block with placeholder ids in it was borrowing the dark ground that means "the binary
  wrote these bytes"; the copy recommended `isu ready --json | head -1` beside a card proving
  the flag is a no-op; and the type scale's comment contradicted its own six ratios. `gateProse`
  and the `ran`/`sketch` grounds came out of it.
- Round two: twelve scrolling regions across the site and not one `tabindex` — WCAG 2.1.1, in
  the element this site is built around. `gateFocus` reads the stylesheet rather than a list of
  element names, and **a scrolling selector it cannot evaluate fails the build**, because the
  bug was not the missing rule but that the gate did not know what it was not checking.
- Round three: the share card was 1200×630 of dark ground with a logo on it and no argument,
  which is the one asset that reaches a reader before the page does. The page's only call to
  action was a command with two `aria-hidden` spans round it, so selecting and pasting it gave
  `$ go install …@latest_` — `aria-hidden` hides a string from a screen reader and not from a
  clipboard. Four of six sections named a documentation page and did not link it. And the middle
  verb of the headline was the one the page never demonstrated.

What each of those produced is in the diff: a card that sets the tagline out of `CONTENT.md`, a
prompt and caret drawn by the stylesheet with `gateDecoration` refusing text inside anything
`aria-hidden`, five links where there were two, and a section that shows the loop closing rather
than four snapshots of a state machine.

**A fourth read found the one place the front page and this repository's own evidence
disagreed.** Section 2 said "two people cannot both take the same issue" while
`docs/field-notes.md` says, in as many words, that the board reads local refs only and a
colleague's claim is therefore invisible. Both are true — the *lock* holds, because the second
push is refused; the *warning* does not — and the landing page was making the multiplayer claim
without the carve-out, on the one page of eight it did not link. The carve-out is in section 2
now, with the link, which is the move section 5 already makes with `not-doing` and gets credit
for.

**A section carries a sequence of samples rather than one.** `Section.Samples` is a slice because
a state machine is not proved by a snapshot of it: the merge is now the same command on two
claims one merge apart, `in progress` and then `done`, with no flag on either. Getting there
needed a fixture that contains a completed loop, which it did not — every issue in it was either
finished before the samples start or still in flight — so `APP-b5n3kt` is claimed on a branch and
then squashed onto trunk, and the branch is left standing because that is what happens to
branches.

**`--ref <an ancestor of trunk>` is not a way to show a merge, and that is a product
observation.** Reading the fixture at `main~1` reports `in progress (contended)` with two
claimants, because `refs/heads/main` is a ref that proposes `resolved` and the contention rule
counts it. It is correct and it is unreadable on a landing page, which is why the two cards are
flagless. M3's contention rule deciding that trunk-ahead-of-the-ref is not a rival claimant is a
story of its own.

**The tutorial cannot quote a claim, and now says so instead of ending in a sketch nobody
explains.** `isu claim` writes a random `Isu-Claim:` nonce, so the commit id differs every run;
the board after it reads `remote refs just now`; and the claimant it prints is whoever `git` is
configured as on the machine that built the page — `claimed by Claude`, on the run that found
this. A page whose bytes must be identical on every machine can quote none of those three, so
`docs/getting-started.md` names the reason and sends the reader to the front page, where the
clock and the identities are fixed. Its lede promised "a merged fix" and now promises what it
delivers.

**Accessibility, twice over.** The fix that made every scrolling box reachable gave each one
`role="region"`, which makes it a *landmark*: `docs/json.html` announced sixteen landmarks all
called "table". They are `role="group"` now — reachable, labelled on entry, out of the landmark
list — and a block that did not run says "example, not run" rather than "terminal output", which
is the distinction the stylesheet had been drawing since the round before and the accessibility
tree had never heard of. And the five cards on the landing page were reachable while announcing
nothing on focus: no rule covered `.proof pre`, so it fell back to the browser's outline, which
`.proof`'s own `overflow: hidden` clipped on three sides. The ring is inset now, in `--glow`,
which is 9.09:1 on the terminal ground.

**The install line was 40% off the right-hand edge of a phone.** `gateWidth` could not see it:
that gate holds `<pre>` and `<table>` to a scrolling box, and the install line is a `<p>`, so
"no page-level horizontal scroll" and "readable on a phone" came apart exactly where the page
asks somebody to type something. It wraps under 30rem and `user-select: all` makes one tap take
the command and neither pseudo-element — verified in Chromium, along with the focus ring and the
landmark counts.

**The share card is 1200×630 and had three letters on it.** It sets the tagline now, read out of
`CONTENT.md` so the card and the page cannot disagree, which cost the bitmap face an alphabet —
five by nine, the last two rows for descenders, and a tagline carrying a character the face
cannot set fails the build rather than drawing a hole.

**And `TokensAgree` exists because this document lied about itself twice.** The plan's colour and
type tables are prose about a stylesheet, and prose about a file stops being true: the type table
went on saying `--text-l` was `1.25rem` for a week after it became a clamp. Every row naming a
token is now held to what the stylesheet declares, in both schemes.

**Two findings were not acted on, and the reason is the same in both cases: they are somebody
else's story.**

- **`isu board` prints `done` and `dropped` first.** `internal/cli/view.go` renders the groups in
  `model.Statuses` order, and that slice is the *precedence* table from §1 — which rule wins when
  two match. That has nothing to do with what a person wants to read first, so the flagship card
  on the landing page opens with three rows of finished work, one of them a joke about rewriting
  the CSS. It is a real defect and it is M2/M3's, not M8's; a display order of `open`,
  `in progress`, `awaiting triage`, `reopened`, then the terminal groups is the obvious fix and
  it moves golden files in `internal/cli` that this pull request has no business moving.
- **The `isu ready` card is two 22-field objects with `"question":"","reason":"","resolution":""`
  visible in both.** The complaint is fair — it reads as "this JSON is mostly empty" — but two
  lines is what makes *newline-delimited* legible, and `isu ready` has no flag that would print
  one. Filling those fields in the fixture would mean inventing content for states the issues are
  not in, which is the one thing this site may not do.

`SiteURL` is the only absolute URL the site contains. It was
`https://dgorshkov.github.io/isu` while this story was written and is
`https://isu-website.netlify.app` now, decided under M8-S3. Everything else is relative, so the
site works from a `file://` checkout, from a deploy preview at a URL nobody chose and from a
domain of its own without being rebuilt — changing the host is one constant and a `make site`.
**Branch** `isu/M8-S2-landing`
**Build** the page from `CONTENT.md`. Colours and type sizes come from CSS custom properties
declared once; fonts are self-hosted and subset, no third-party font CDN. Terminal output, board
renders and check results are produced by running `isu` against a fixture repo at build time and
embedded — never authored by hand.
**Tests first** a test regenerating every sample and failing if the committed page differs —
this is what stops the site drifting from the product; a test asserting no template hardcodes a
hex value or a pixel font size; a test asserting the built HTML makes zero third-party network
requests.
**Done when** every artifact on the page came out of the binary in this repo.

### M8-S3 · Docs, gates and deploy ✅
**Done** #21, 2026-09-09. Eight pages under `docs/`, every console block in them run against a
scratch repository during `make test` and `make site`. Two facts about them are worth writing
down rather than discovering:

- **`docs/importing.md` documents a milestone that does not exist yet.** M7 has not been built,
  so the page opens by saying the command is not in this build, publishes the mapping the plan
  specifies, and contains no `$ isu` line at all — there is nothing to run and it does not
  pretend there is. It is the honest version of a page M8-S3 asks for and M7 has not earned.
- **`docs/getting-started.md` runs against a repository it is allowed to write to**, so `isu
  init` and `isu new` actually run. What it cannot assert is anything containing a generated id,
  because an id is thirty bits of hash over eight random bytes; those blocks run and their output
  is not compared, and the page says so where it quotes one.

**`docs/importing.md` is a debt this milestone hands to M7, and the next session should collect
it.** M7 is written and open as #15 against the same base as this pull request; the two were
merged in the order the reviewer chose, this one first, so the moment #15 lands that page's
first paragraph is false and its mapping is documentation of something that ships. What it owes
is small and specific: drop the "not in this build" note, and give the page ```console blocks
running `isu import github` against a recorded dump, so the importer's documentation executes
like every other page here. Merging #15 will also conflict in this file — both pull requests
mark a milestone done in the same table and add `**Done**` paragraphs a few lines apart.

**Netlify was rewriting the pages CI had just verified, and nothing in this repository could
have told anybody.** Pretty URLs post-processing is on by default and is a dashboard form, so
every deploy preview served an `index.html` 46 bytes shorter than the committed one — every
internal href rewritten from `docs/json.html` to `/docs/json`, every attribute requoted from `"`
to `'`. `site.css` and `og.png` came through untouched; only HTML was changed. The third
consequence is the one that matters: twelve gates run inside `Build` over the bytes in
`web/site`, and not one of them had ever seen a byte a reader was served — `gateLinks` proved
`docs/json.html` resolves, and the reader got `/docs/json`. `netlify.toml` says
`skip_processing = true` now, with the reasoning in the file, and `scripts/site_test.go` asserts
that block is there. That assertion is as far as a test in this repository can follow the bytes:
**nothing here gates the delivery**, and the honest way to close that would be a check that
fetches the deployed page and diffs it against `web/site` — which needs a deploy to exist and is
a story of its own.

**The gates are hand-written over the built site, and what they can and cannot see is stated in
`internal/site/gates.go`.** There is no browser in this build, so "no horizontal scroll at
360 px" is enforced as the two things that cause it — a fixed width wider than the viewport, and
wide content outside a box that scrolls — rather than measured. That claim was checked once
against a real Chromium at 360 px while the story was being built, and the page's `scrollWidth`
equalled its `clientWidth`; the gate that runs on every build is the proxy, and it is a proxy on
purpose rather than a browser dependency in the allowlist.

The markdown renderer is `internal/site/markdown.go` and it is not the thing the out-of-scope
list drops. It renders a fixed subset, refuses a line it does not understand, never passes raw
HTML through — a `<script>` in a source document is escaped and rendered as text — and is never
handed an issue body. There is nothing for a sanitiser to do and no configuration in which
there would be.

**Netlify publishes the site, and `.github/workflows/site.yml` publishes nothing.** The story
asks for publishing on merge to trunk from CI and for a workflow lint asserting the deploy job
triggers only on trunk; the site was already wired to Netlify while this milestone was being
built, so the deploy job would have been a second publisher racing the first. What the workflow
does instead is the half that makes the site reviewable: it regenerates web/site on every pull
request and `git diff --exit-code -- web/site` holds the committed bytes to the built ones, and
it runs every console block in the docs while it is there.

The lint changed subject with it, and the replacement is stronger in one direction and weaker in
another — both are worth stating. Stronger: the workflow now holds no write permission at all,
which `scripts/site_test.go` asserts line by line, so nothing it runs on a pull request from
anywhere can reach the address people read. **Weaker: the guarantee that production comes from
trunk left this repository with the deploy job.** It is Netlify's production-branch setting now,
and no test here can see it. `netlify.toml` pins everything that can be pinned in a file — the
publish directory, asserted against the directory `make site` writes, and the absence of a build
command — and the branch is not one of them. A reviewer who wants that guarantee back wants the
Pages job back, and this paragraph is where the trade was made.

`SiteURL` is `https://isu-website.netlify.app`, the one absolute URL on the site.

**One statement in this package is uncovered and is argued for**, in the shape the definition of
done asks for: `documents` propagating a failure from `Renderer.Page`. Every other error return
here is reached — by a source file that is not there, by a stylesheet with no tokens in it, by a
directory something else is sitting on, and by a git on `PATH` that refuses one invocation — but
that one needs `html/template` to fail on a `docBody` this package built out of its own types,
which it cannot. The renderer's own execution failure *is* reached, directly, in
`render_test.go`.
**Branch** `isu/M8-S3-docs-deploy`
**Build** `docs/`: getting started, the data model, every derived status with its rule, the
check catalogue, the JSON contract, importing from GitHub Issues, and a page on what isu
deliberately does not do. Then `make site` producing the whole site from a clean checkout, the
gates —
internal link checker, HTML validity, a 300 KB per-page weight budget, an accessibility pass,
responsive down to 360 px, `prefers-reduced-motion` honoured — and publishing on merge to trunk
from CI, with favicon, Open Graph and Twitter cards, sitemap, canonical URLs and a
404 page that is useful rather than decorative.
**Tests first** a test extracting every fenced shell block from the docs and running it against
a scratch repo — documentation that does not execute is documentation that rots, and these docs
will be read by agents; the budget and accessibility tests fail on any violation; a viewport
test asserts no horizontal scroll at 360 px; a workflow-lint test asserts the deploy job
triggers only on trunk; a test asserts every page has a title, a description and an OG image.
**Done when** every command in the docs actually runs and the site is live from CI.

---

# M9 · Release

### M9-S1 · Cross-platform build
**Branch** `isu/M9-S1-goreleaser`
**Build** goreleaser for linux/darwin on amd64 and arm64, static, reproducible, with version
and commit stamped in.
**Tests first** a smoke test running each built binary's `--version` under emulation where
available. Plus the case M0-S1's `TestVersionCommand` structurally cannot reach: build with
`-ldflags -X main.version=...` and assert the **stamped** value still satisfies the semver
pattern. That test links against the compile-time default and so only ever exercises
`0.1.0-dev`; a release that stamps a leading `v` would ship broken past a green suite.
**Done when** `goreleaser release --snapshot` produces working binaries, and a bad stamp fails
the build rather than the user.

### M9-S2 · Distribution
**Branch** `isu/M9-S2-distribution`
**Build** homebrew tap formula, `go install` path verified, checksums and signatures.
**Tests first** a test installing from the built tarball into a temp prefix and running
`isu --version`.
**Done when** `brew install isu` works from a clean machine.

### M9-S3 · Release candidate hardening
**Branch** `isu/M9-S3-hardening`
**Build** no new features. Fix what the following surface: run `isu` against three real
repositories of different shapes; run the full lifecycle under a squash-merge-only repo; run
with the remote detached for the whole session; run with 20,000 issues; and run with **500 live
branches**, because ref count is the dimension the board's cost actually scales in and the one
least exercised by everything above it.
**Tests first** convert every defect found into a regression test before fixing it.
**Done when** all five scenarios pass and coverage is at or above the gate.

### M9-S4 · v1.0.0
**Branch** `isu/M9-S4-v1`
**Build** CHANGELOG, README final pass, tag `v1.0.0`.
**Done when** the tag is pushed and the release artifacts are attached.

---

## Out of scope for v1.0.0

State this in the README so nobody has to ask:

- **The local web UI** (`isu serve`). v1.0.0 is a CLI and a TUI. Dropping it also drops the
  markdown-to-HTML pipeline, the HTML sanitiser and the XSS surface that came with them —
  `glamour` renders to a terminal, where a `<script>` tag is just text.
- **Jira and Linear import.** v1.0.0 imports from GitHub Issues and nothing else. Whoever is
  adopting isu is already in a git repository, that repository is almost always on GitHub, and
  GitHub answers for free — with no licence and no admin — what Jira answers through an
  integration somebody installed. One importer, done properly, beats three done at once, and
  M7-S1's source interface is the seam the next one arrives through.
- **The WebAssembly derivation demo.** The most convincing possible proof of the central claim,
  and pure marketing: it gates nothing and it is the hardest thing on the website.
- **isu Tower** — the hosted app for people without a clone. Separate repo, after v1.
- Cross-repo issues. Monorepo-first is a position, not an omission — and it is what keeps
  M7-S4's `<PREFIX>-<number>` ids from colliding, since two repositories' `#1` never meet.
- **Sub-issue hierarchies.** isu has one `parent`, it must name an epic, and M7-S4 spends it on
  the milestone. A GitHub tree eight levels deep is recorded whole in `source.yml` rather than
  flattened into a shape that would misrepresent it, which makes modelling it in v1.1 a story
  rather than a re-import.
- **Downloading imported attachments.** GitHub's assets want a browser session on a private
  repository, so M7-S5 records the links and leaves them resolvable where they already live.
- A `fixed/` archive directory. Resolved issues stay in `issues/`.
- Sprints, story points, burndown, time tracking.
- Bidirectional sync with anything. Import is one-way and one-time by design.
- Notifications and email.
- **A second forge.** This file used to ship a GitLab pipeline beside the GitHub one and read
  *CI on both forges*; the project is on GitHub, so a pipeline nobody runs was a pipeline
  that would rot while claiming to be a second opinion. Dropped in #4, along with the parts
  of the design that leaned on it. Nothing above needs a second forge to be true — the
  squash-merge tiers stand on GitHub's own squash settings — so adding one later is work,
  not a redesign.

The first three are deferrals, not rejections, and each has a milestone's worth of design
already written in this file's history.

## Definition of done, every story

This list is per story, not per pull request: one carrying several stories satisfies all of it
for each of them separately.

1. The failing test is its own commit, and it precedes the commit that makes it pass.
2. `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./...` all pass at the
   branch head.
3. Coverage is at or above **99% across `./...`** — the whole module, `cmd/` and the test
   harness included — and 100% for `internal/model`.

   **Raised from 85% in #8**, where measuring it found the floor was not a gate at all: the
   tree stood at 96.9%, so twelve points of coverage could have been deleted without CI
   noticing, and the number had never once been the thing that caught anything. Closing what
   the measurement exposed — six untested behaviours in the loader, and every error return
   nobody had ever taken — carried it to 99.2%.

   **99% counts `internal/gittest` too, and that was argued about first.** The harness fails
   the test rather than returning an error, so its refusals cannot be exercised the ordinary
   way: calling one from a test fails that test. They are driven instead through a counterfeit
   `testing.TB` that records the refusal and stops. The case for exempting the harness is that
   this is machinery built to move a number. The case that won is that a harness nobody has
   ever seen refuse is a harness whose refusals are a comment, and every fixture in this
   project trusts them.

   **The headroom was three statements when this was written**, so this floor will be the thing
   that fails a story sooner or later, and that is the point of it. Eleven statements remained
   uncovered, and each one needs the filesystem to fail underneath it: `main` calling `os.Exit`; the loader's
   `os.Stat` and `os.ReadFile` on a path it has just walked to (four); the harness failing to
   open, write or close the `.git/config` it has just initialised (four); the harness's
   `gitx.New` on a fresh `TempDir`; and `os.Rename` moving a remote aside. A story that adds an
   error path it cannot reach should expect to argue for it.

   **M4 is what "sooner or later" looked like.** The milestone took the module from 1,375
   statements to 2,619 and arrived at 94%, which is 158 uncovered against a budget of 26.
   Closing it to **99.2%** took a fourth of this milestone's effort and was worth every hour of
   it: two of the things it made somebody look at were defects rather than missing tests — `isu
   triage` reading the issue at trunk, and a repository whose trunk is called neither main nor
   master being unable to resolve anything — and two dead functions were deleted rather than
   tested, because a function nobody calls is not coverage owed.

   Three levers reach the error returns, and none of them depends on the suite running
   unprivileged, which matters because it runs as root often enough that a `chmod` proves
   nothing: a directory that is not a repository; a path something else is already sitting on (a
   file where a folder belongs, a directory where a file belongs); and **a git on PATH that
   forwards to the real one and refuses exactly one invocation**, counted, so that
   `git log --first-parent` can fail without the `git log --reverse` beside it. That last is the
   shim M2-S1's own tests already use, pointed at the whole product rather than one wrapper, and
   it is how "what does isu say when git will not answer" became a thing this project asserts
   rather than hopes.

   Eighteen statements stayed uncovered and were argued for: the eleven above, plus `repo.Open`
   on a directory `rev-parse --show-toplevel` has just named, `os.WriteFile` on two paths their
   functions have just made, `cat-file --batch` failing after the `ls-tree` that named its
   objects did not, a base git resolves for the load and refuses two processes later, and
   `config.NewID` failing — which it cannot, since the generator reads no file and takes no
   lock, but its signature says it might.

   **M6 took the module to 4,009 statements and left thirty-one uncovered, which is 99.2% and
   nine statements of headroom.** Six of those are this milestone's, and the shape of the argument
   is the one below: two are glamour refusing to build a renderer or to render — a style name this
   package chose and a width it computed, so neither can fail on anything a user did, and the body
   is printed as it was written rather than lost; two are the checkout `g` performs, where git
   answers something other than "no such revision" and where the switch itself fails; and two are
   `isu new`'s own pair, git having no `user.name` and an id generator that has stopped being
   random, reached through the editor path this milestone added.

   **The floor did what this section says it is for, twice in one milestone.** Both things it made
   somebody look at were defects rather than missing tests: a fold measured its width in characters
   and counted escape sequences among them, so every coloured line would have folded early on a
   terminal and never in a test; and `n` opened the editor before deriving the repository, so an
   editor somebody spent ten minutes in could close on "isu cannot read that ref".

   **M5 took the module to 3,304 statements and left twenty-five uncovered, which is 99.2% and
   eight statements of headroom.** Five of those are this milestone's and each is argued for
   the same way: `isu init` failing to make a directory or to change the mode of a file it has
   just written; the two git answers that name a repository's hooks directory, and the relative
   name of one that `core.hooksPath` put outside the working tree; and `os.DirEntry.Info` on a
   file the walk beside it has just listed. Everything else the milestone added is reached,
   most of it through the git shim above — including, now, the loader that reads what a branch
   proposes, whose three processes are refused one at a time by counting.
4. The story's own issue file is flipped to `resolved` in the same pull request (from M5-S7,
   which is where issue files start existing).
5. The pull request describes what changed, what was decided, and anything that needs a call.
6. The same pull request marks the story done in this file — heading ✅, `**Done**` line,
   milestone status column, per §0.
7. You have not merged it.
