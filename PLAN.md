# isu v1.0.0 — build plan

`isu` is an issue tracker with no database. Issues are folders inside the repo; a pull
request that fixes a bug also closes it, in the same diff. Written in Go, shipped as one
binary containing a CLI and a TUI.

This document is the build order. Work it top to bottom, one story per branch, one pull
request per story. Do not skip ahead, do not batch stories, do not merge your own work.

---

## 0. Working agreement

**Read this section before every session.**

- **One story = one branch = one pull request.** Branch name is the story id, slugged:
  `isu/M3-S2-derive-epic-rollup`.
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
- **Name the reviewer before M0-S1.** Fifty-one pull requests arrive one at a time and none of
  them merge without a human. The schedule below is that person's calendar, not the agent's.
- **Mark it done in this file, in the same pull request.** The pull request that finishes a
  story also updates PLAN.md, or the story is not finished. Specifically:
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
| Language | Go 1.23+ |
| CLI | `spf13/cobra` |
| TUI | `charmbracelet/bubbletea`, `bubbles`, `lipgloss`, `glamour` |
| TUI tests | `charmbracelet/x/exp/teatest` |
| YAML | `goccy/go-yaml` — for `.isu.yml` and `source.yml`, not for frontmatter |
| Assertions | `stretchr/testify/require` |
| Release | `goreleaser` |

That is the entire dependency allowlist. Adding anything else requires asking first.

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
a `git` command.

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
parent: AR-40b1cc      optional, must name an issue of type: epic
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
claim ref, not by this field.

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
claim ref still exists matches both `done` and `in progress`.

| status | rule |
|---|---|
| `done` | trunk has `state: resolved` |
| `dropped` | trunk has `state: dropped` |
| `awaiting triage` | folder exists on a branch, not on trunk |
| `in progress` | a claim ref exists, or some branch has `state: resolved` where trunk has `open` |
| `reopened` | trunk has `state: open`, and some earlier trunk commit had `resolved` |
| `open` | on trunk with `state: open`, nothing claims it |

Terminal trunk state beats every claim, so a finished issue reads `done` whether or not its
claim ref was tidied up. `in progress` beats `reopened` because someone actively re-fixing an
issue needs to show as worked, not as merely broken again — but **reopened survives as an
annotation** on whatever status wins, so the fact is never lost.

`in progress` has two sources because claims are advisory: someone who never ran `isu claim`
but has pushed a branch that resolves the issue is, observably, working on it.

Annotations on `in progress`: **claimant** (author of the claim commit), **age** (claim commit
date, or branch tip date when there is no claim), **stale** (older than 7 days), **contended**
(a claim ref plus a branch by someone else, or two branches, resolving the same issue).

An issue of `type: epic` takes its status from a fold over its children. All children terminal
→ resolved, unless all are dropped → dropped. Otherwise open. An epic with no children is a
check failure, not a status.

### IDs

`<PREFIX>-<token>`, prefix from `.isu.yml`. The token is the first six characters of Crockford
base32 over `sha256(title ‖ owner ‖ created ‖ 8 bytes from crypto/rand)` — lowercase, with
`i`, `l`, `o` and `u` excluded from the alphabet so nothing is ambiguous read aloud or typed
from a screenshot.

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

### Configuration

`.isu.yml` sits at the repository root. This is the whole schema; stories below may not invent
keys, and an unknown key is a validation error rather than a silent no-op.

| key | type | default | meaning |
|---|---|---|---|
| `prefix` | string | — | required; the issue id prefix |
| `agents` | list of strings | empty | commit authors treated as agents by M5-S4 |
| `direct_triage` | bool | `false` | allow `isu triage --push` to write straight to trunk |
| `stale_days` | int | `7` | a claim older than this is stale |
| `fetch_warn_hours` | int | `24` | warn when the newest remote ref is older than this |
| `attachment_max_bytes` | int | `524288` | per-attachment cap enforced by M5-S2 |

Every one of these is read by a story below, so the file's schema is validated in M1-S4 rather
than discovered a milestone at a time.

### Claims

`isu claim AR-7f3akq` does, in this order:

1. build a **parentless commit with an empty tree**, authored by the claimant, subject
   `claim AR-7f3akq`, and `git push origin <sha>:refs/claims/AR-7f3akq`. **Do this first**,
   before any work, so the loser wastes nothing.
2. create branch `isu/AR-7f3akq`. **The issue file is not touched.**
3. push the branch.

The parentless commit is what makes step 1 a real compare-and-swap. A commit with no parents
can never be an ancestor of whatever is already at that ref, so a second claimant's push is
always rejected. Pushing an existing sha — a trunk tip, say — would not be: git would accept it
as a fast-forward, or as a no-op if both claimants pushed the same commit, and both would
believe they had won.

It also carries the answers M3-S3 needs. The claimant is the commit's author and the claim time
is its author date, read directly rather than reconstructed by hunting for the commit that
flipped a field.

Claiming does not mark anything resolved. `isu resolve` is the only command that writes
`state: resolved`, so a branch that claims an issue and does no work fails the M5-S3 evidence
check exactly as it should.

`isu unclaim` deletes the claim ref. Claims are advisory for humans and binding for agents.

**A claim is released when the work lands.** `isu unclaim` does it by hand; the CI template from
M5-S6 deletes `refs/claims/<ID>` on every push to trunk that leaves the issue terminal. Nothing
else in this design ever removes a claim ref, so without that sweep every completed issue keeps
one forever and M5-S5 reports it as a stale claim for the life of the repository. The status
precedence above keeps the board correct in the meantime; the sweep is what keeps the warnings
correct.

Some remotes refuse pushes outside `refs/heads/*` and `refs/tags/*`. `isu claim` must tell that
rejection apart from a lost race and say so plainly — the two look identical in git's output and
mean opposite things. `--no-claim` is the documented degraded mode: branch, work, and let
contention surface at pull-request time instead.

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
| M1 | Issue files | parse, serialise, validate, version | not started |
| M2 | Git layer | fast load from any ref, from the working tree, and from trunk history | not started |
| M3 | Derivation | statuses, epics, claims, contention | not started |
| M4 | CLI | board, show, ready, new, claim, resolve, drop, comment, triage, field notes | not started |
| M5 | Checks | `isu check`, hooks, GitHub Actions, dogfooding | not started |
| M6 | TUI | `isu ui` | not started |
| M7 | Importers | safe writes, Jira | not started |
| M8 | Public website | content, landing page, docs, deploy | not started |
| M9 | Release | goreleaser, brew, docs, v1.0.0 | not started |

### Sizing

Estimated in pull requests, because the reviewer is the constraint and the compiler is not.

| milestone | PRs | shape |
|---|---|---|
| M0 Foundations | 4 | small, mostly config; M0-S4 is the one that matters |
| M1 Issue files | 5 | small, pure functions, heavy table tests |
| M2 Git layer | 5 | medium; M2-S2 is the trickiest parsing in the project |
| M3 Derivation | 5 | medium; this is the product, expect the most review time here |
| M4 CLI | 8 | medium; M4-S8 has no code and may generate several follow-ups |
| M5 Checks | 7 | small each, and highly parallel in principle |
| M6 TUI | 5 | medium; golden-frame tests are fiddly to stabilise |
| M7 Importers | 5 | large; M7-S5 is the biggest single PR in the plan |
| M8 Website | 3 | medium |
| M9 Release | 4 | small, except M9-S3 which is open-ended by design |

**51 pull requests.** At three reviewed per day that is roughly four weeks; at one per day,
roughly eleven. The agent is not the bottleneck — plan your own calendar, not its.

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
**Done** #4, 2026-08-24. The profile is built with `-coverpkg=./...`, without which a
package carrying no test file of its own is absent from the profile and raises the
average by being untested.
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
**Done** #4, 2026-08-24. **This story was amended as it was built.** It read *CI on both
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
**Done** #4, 2026-08-24. Commits by a second author and claim refs are not in the
harness yet; they arrive with M3-S3, which is where what they mean is decided.
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

# M1 · Issue files

### M1-S1 · Frontmatter parser
**Branch** `isu/M1-S1-frontmatter`
**Build** `internal/issue`: parse `---` delimited key/value frontmatter plus body. Unknown
keys are preserved verbatim on round-trip. Parsing never panics on malformed input; it
returns a typed error with line number.
**Tests first** table-driven: valid, missing close delimiter, duplicate key, empty file, CRLF
line endings, unicode values, a 1 MB body. Plus a round-trip property test: parse → serialise
→ parse yields an identical struct.
**Done when** the round-trip test passes on every fixture in `testdata/issues/`.

### M1-S2 · The Issue type and its schema
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

### M1-S3 · Reading and writing an issue folder
**Branch** `isu/M1-S3-folder-io`
**Build** load an issue from `issues/<ID>/`, listing attachments and `comments/`. Write an
issue back, preserving unknown keys and body byte-for-byte where unchanged.
**Tests first** a folder with attachments and three comments loads with all of them; writing
an unmodified issue produces a zero-length diff (assert with `git diff --exit-code`).
**Done when** the zero-diff test passes. This property matters more than it looks: it is what
keeps pull requests readable.

### M1-S4 · ID generation and `.isu.yml`
**Branch** `isu/M1-S4-ids`
**Build** config loading and validation against the `.isu.yml` table in section 1, plus
`NewID(title, owner, created)` implementing the token scheme. **`NewID` is pure** — it reads
nothing, knows about no other issue, and cannot detect a collision, because detecting one means
loading the repository and the git layer does not exist until M2. No counter, no scan for a
maximum, no `isu renumber` — ids are permanent from creation.
**Tests first** the same inputs plus different random bytes give different ids; the alphabet
never emits `i`, `l`, `o` or `u`; `NewID` issues zero reads against a 5,000-issue fixture; an
imported `PROJ-1234` validates as an id even though nothing would ever generate it; every key
in the config table round-trips with its default applied, an unknown key is refused, and a bad
value is refused with the key named.
**Done when** generating an id needs neither the network nor a read of `issues/`. Collision
regeneration is deliberately **not** here — it needs the loaded repo, so it belongs to `isu new`
in M4-S3.

### M1-S5 · Schema version and migration
**Branch** `isu/M1-S5-schema-version`
**Why** `schema:` is the promise that v1.0.0 is not a format prison, and an untested promise is
decoration. This story is what makes the field real, and it is cheap now and expensive after
people have repositories.
**Build** version gating in the reader: a known `schema:` loads, an unknown one is refused with
a message naming the version of `isu` that understands it and the version it found. Plus a
`Migration` interface and the registry that dispatches on version, with zero migrations
registered.
**Tests first** `schema: 1` loads; `schema: 2` is refused and the error names both versions;
`schema:` missing is refused; `schema: banana` is refused with a parse error rather than a
panic; a registered no-op migration from a fixture at version 0 produces a byte-identical
version 1 file.
**Done when** a `schema: 2` repository fails with an error a human can act on, proven by a test
rather than by inspection.

---

# M2 · Git layer

### M2-S1 · `internal/gitx`
**Branch** `isu/M2-S1-gitx`
**Build** the only place that executes `git`. Typed wrappers for `ls-tree`, `cat-file --batch`,
`log`, `rev-parse`, `for-each-ref`, `diff --name-only`, `push`, `show`. Context-aware, with
timeouts. Errors carry stderr. Detects git absence at startup with a clear message.
**Tests first** each wrapper against a `gittest` repo; an error case per wrapper; a test that
asserts the binary is invoked with `--no-pager` and a clean environment.
**Done when** `grep -r "exec.Command" internal/ | grep -v gitx` returns nothing. Add that
grep as a test.

### M2-S2 · Load every issue from a ref
**Branch** `isu/M2-S2-load`
**Build** `repo.LoadRef(ref) map[string]Issue` using the mandated read path: `ls-tree -r` for
object ids, one `cat-file --batch` fed object ids, stream-parse the output.
**Tests first** load from trunk, from a branch, from a detached SHA, from an empty repo;
issues present on one ref and absent on another; a blob containing the batch delimiter
sequence in its body (this will break a naive parser — write that test first).
**Done when** loading is correct on all of the above.

### M2-S3 · Load from the working tree
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

### M2-S4 · The trunk history index
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

### M2-S5 · The performance gate
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

# M3 · Derivation

### M3-S1 · Status derivation
**Branch** `isu/M3-S1-status`
**Build** `internal/model`: given trunk, every branch, the claim refs and the history index
from M2-S4, derive the six statuses from the table in section 1. **Pure function over loaded
inputs — no git calls inside, and no exceptions to that rule later.** Everything a status needs
is loaded by M2 and passed in; if a future status needs something else, the loader grows, not
this package.
**Tests first** one test per status, then the transitions between them. Include: issue on two
branches, issue on a branch identical to trunk, branch deleted after merge, a claim ref with no
branch, a branch with no claim ref.
**Done when** the status function has 100% branch coverage. This package is the product; it
gets a higher bar than the rest.

### M3-S2 · Epic rollup
**Branch** `isu/M3-S2-epics`
**Build** children indexed once per load, not scanned per parent. Fold child states into every
`type: epic`. Cycle-safe: a parent cycle must return a value, never recurse forever.
**Tests first** rollup with mixed children; all dropped; nested epics three deep; **an epic
that is its own parent**; a two-node parent cycle; an epic with no children; a `parent:` naming
an issue that is not an epic. The self-parent case blew a stack during prototyping — write it
before the implementation.
**Done when** a 5,000-issue fixture rolls up 100 epics in under 50 ms.

### M3-S3 · Claimant, age, staleness, contention
**Branch** `isu/M3-S3-claims`
**Build** read `refs/claims/*` in one `for-each-ref`. The claim commit's author is the claimant
and its author date is the claim time; staleness is that date against the threshold. An issue is
contended when more than one ref resolves it — two claim refs cannot coexist, so contention in
practice means a claim ref plus a branch someone else pushed, or two such branches. A branch
resolving an issue with no claim ref still counts as `in progress`, with the branch tip date
standing in for the claim time.
**Tests first** single claim; a claim plus a foreign branch (assert both are named); a claim
backdated 11 days reads as stale; an unclaimed resolving branch reads as in progress with the
tip date; a claim ref whose branch was deleted reads as stale rather than vanishing.
**Done when** contention and staleness come out of the same single scan, and nothing in this
story needs to hunt for the commit that changed a field.

### M3-S4 · Reopen detection
**Branch** `isu/M3-S4-reopen`
**Build** a **pure fold over the M2-S4 history index**: if any earlier entry was
`state: resolved` and the latest is `open`, the status is `reopened`. No git in this package —
that is the whole reason M2-S4 exists.
**Tests first** merge then revert reads as reopened; merge, revert, re-fix reads as done;
never-resolved reads as open; **and the same three with the branch deleted after merge**. Plus
a test asserting this package spawns no processes, in the style of the M2-S1 grep test.
**Done when** reopen is correct with zero branches left in the repo.

### M3-S5 · Squash-merge lifecycle
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

# M4 · CLI

### M4-S1 · Command scaffold and output contract
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
**Done when** adding a command without `--json` fails the test suite.

### M4-S2 · `isu board` and `isu show`
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

### M4-S3 · `isu new` and `isu ready`
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

### M4-S4 · `isu claim` and `isu unclaim`
**Branch** `isu/M4-S4-claim`
**Build** the three-step claim from section 1, in that order — parentless claim commit pushed
to `refs/claims/<ID>` first, then the branch, and **no write to the issue file**. Claim failure
must be fast, quiet and exit non-zero with a machine-readable reason that distinguishes a lost
race from a remote that refuses the ref namespace. Build `--no-claim` here too — section 1
names it the documented degraded mode for those remotes, and a documented mode that no story
builds is a promise the plan breaks.
**Tests first** the rejection path **deterministically**: create the claim ref, then claim
from a second clone and assert the failure names the holder and exits non-zero. **Assert the
claim commit is parentless**, and assert a claim attempt built on the current trunk tip is
still rejected — that is the case a fast-forward would silently let through. Repeating a
network operation a hundred times per CI run buys confidence in the network, not the code — so
the hundred-run stress variant lives behind `//go:build stress` and is out of the default
suite. Also: claiming leaves the issue file byte-identical; unclaim releases; claiming an
already-terminal issue is refused; a remote rejecting `refs/claims/*` produces the namespace
error and not the race error; **claiming with the remote detached fails without leaving a local
branch behind**.
**Done when** the deterministic rejection test passes and `make stress` exists for the rest.

### M4-S5 · `isu resolve` and `isu drop`
**Branch** `isu/M4-S5-resolve-drop`
**Build** flip state on the current branch. `resolve` writes `state: resolved` and an
`Isu-Resolves: <ID>` trailer on its commit. `drop` requires `--reason` and `--resolution`.
Neither command merges anything; both leave a branch for a pull request.
**Tests first** resolve on a spike without an artifact warns; resolve writes the trailer and it
survives a squash merge; drop without a reason is refused; drop without a resolution is
refused; both refuse to run directly on trunk; **resolve on a freshly claimed issue is the
first thing that writes `resolved` to the file** — the claim did not.
**Done when** the only way to reach `done` is a merged pull request.

### M4-S6 · `isu comment`
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

### M4-S7 · `isu triage`
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

### M4-S8 · First contact with a real repository
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

# M5 · Checks

### M5-S1 · The check engine
**Branch** `isu/M5-S1-check-engine`
**Build** `internal/check`: a `Check` interface, a registry, severity levels (`fail`, `warn`),
and a reporter with human and JSON output. Checks receive loaded refs and the diff against
trunk — never raw git.
**Tests first** engine ordering is deterministic; a failing check exits 1, warnings exit 0;
`--json` output validates.
**Done when** adding a check is one file and one registry line.

### M5-S2 · Structural checks
**Branch** `isu/M5-S2-structural-checks`
**Build** schema validity, id matches folder, duplicate ids, `parent` and `blocked_by` exist,
**`parent` names an issue of `type: epic`**, parent cycles, self-parent, dependency cycles,
**an epic declaring its own `state:`**, **an epic with no children**, and the attachment size
cap from `attachment_max_bytes`.
**Tests first** one repo fixture per violation, and one clean fixture asserting zero findings.
The duplicate-id fixture matters more than it used to: it is now the only thing standing
between two clones that generated the same token at the same moment and a corrupt tree.
**Done when** every rule in section 1 that can be checked without a diff is checked.

### M5-S3 · Evidence checks
**Branch** `isu/M5-S3-evidence-checks`
**Build** the type table's resolution rules: resolving requires a change outside `issues/`;
a spike requires an artifact in its own folder; a drop requires `reason` and `resolution`.
A drop that also changes code is a **warn, not a fail** — closing a duplicate in the same pull
request as the fix is a normal thing to do, and refusing it just teaches people to split the
work into two reviews.
Only applies to issues the branch actually changed.
**Tests first** resolved-with-no-code fails; resolved-with-code passes; **a claimed issue whose
file is untouched is not flagged, because claiming no longer writes to it**; spike with only
`README.md` fails; spike with `decision.md` passes; drop without `resolution` fails; drop with
code changes warns and exits 0.
**Done when** an agent cannot mark work done without doing it.

### M5-S4 · Owner immutability
**Branch** `isu/M5-S4-owner-immutability`
**Build** if the commits on this branch change `owner:` and their author is in the configured
`agents:` list in `.isu.yml`, that is a `fail`. Humans may change it freely.
**Tests first** agent-authored owner change fails; human-authored passes; agent changing
`state` but not `owner` passes; a mixed-authorship branch fails on the agent's commit.
**Done when** accountability cannot be reassigned by anything that isn't a person.

### M5-S5 · Contention and staleness reporting
**Branch** `isu/M5-S5-contention`
**Build** warn when another ref claims the same issue, naming the branch and holder; warn on
claims older than `stale_days`. Both warnings are statements about refs, so both are
only as true as the last fetch: run `--fetch` in CI, and include the fetch age in the warning
so a local run that disagrees with CI is self-explaining.
**Tests first** two branches claiming the same issue produces one warning naming the other
branch; stale claim produces a warning with the age in days; a stale local ref set produces a
warning that says so rather than reporting confident nonsense.
**Done when** both surface at pull-request time rather than at merge.

### M5-S6 · Hooks and CI templates
**Branch** `isu/M5-S6-init`
**Build** `isu init` writing a pre-commit hook and `.github/workflows/isu.yml`. Flags
`--hooks`, `--actions`. Idempotent: running twice changes nothing. Never overwrites an existing
file without `--force`.
**Tests first** init into a clean repo produces working files; init twice produces a
zero-length diff; init over an existing workflow without `--force` refuses.
**Done when** the generated pipeline runs `isu check` and fails the build correctly.

### M5-S7 · Dogfooding switch
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
story pull request also flips its own issue file to `resolved` — and M5-S3 turns that into a
failing check if the pull request contains nothing else.

---

# M6 · TUI

### M6-S1 · Shell, layout and key map
**Branch** `isu/M6-S1-tui-shell`
**Build** bubbletea program: header with status counts, list pane, detail pane, footer key
hints. Resize-aware down to 80×24. Keys: `enter` open, `c` claim, `n` new, `r` ready queue,
`g` go to branch, `/` filter, `q` quit.
**Tests first** `teatest` golden frames at 80×24 and 140×40; a resize sequence; `q` quits
cleanly and restores the terminal.
**Done when** golden frames are stable across runs.

### M6-S2 · List and grouping
**Branch** `isu/M6-S2-tui-list`
**Build** the issue list with epics as parents and their children indented beneath, status
colouring, and the counts in the header staying consistent with the list beneath them.
**Tests first** golden frames for a list with nested epics; an epic with forty children; a
list of zero issues rendering the empty state rather than a blank pane.
**Done when** the grouping matches `isu board` exactly, asserted by a shared fixture.

### M6-S3 · Filter and navigation
**Branch** `isu/M6-S3-tui-filter`
**Build** incremental filter across id, title, type, status and owner; vim and arrow keys;
selection preserved across filter changes where the selected issue still matches.
**Tests first** narrowing then clearing restores the previous selection; navigating across a
collapsed epic; filtering to zero results and back; a filter string containing regex
metacharacters is treated literally.
**Done when** filtering a 5,000-issue fixture stays inside one frame budget.

### M6-S4 · Detail pane
**Branch** `isu/M6-S4-tui-detail`
**Build** body rendered with `glamour`, title, priority, claim line, acceptance/repro,
attachments, comment list, epic position, and `blocked_by` showing each blocker's own status.
**Tests first** golden frames for each type; an issue with twenty attachments scrolls; a
contended issue shows both claimants.
**Done when** the detail pane answers "can I start this?" without leaving the TUI.

### M6-S5 · Actions from the TUI
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

### M7-S1 · Import framework and dry run
**Branch** `isu/M7-S1-import-framework`
**Build** `internal/importer`: a source interface, field mapping to the isu schema, an
evidence-tier recorder, and a `--dry-run` report showing counts, coverage and samples without
writing anything. **Source keys become ids verbatim** — `PROJ-1234` stays `PROJ-1234`,
because every link, bookmark and commit message your team has ever written points at it. Unmapped source fields go to `source.yml` in
the issue folder, never into frontmatter — a mature tracker has two hundred custom fields and
they must not poison the schema.
**Tests first** dry run writes no files; mapping preserves keys; a source with two hundred
custom fields produces clean frontmatter and a complete `source.yml`.
**Done when** `--dry-run` is the default and writing requires `--write`.

### M7-S2 · Safe writes
**Branch** `isu/M7-S2-safe-writes`
**Why** An importer writes attacker-influenced data into your repository. Ticket titles,
filenames and attachments all originate outside your control. v1.0.0 never serves that data over
HTTP — the web UI is out of scope, and `glamour` renders to a terminal — so **writing it to disk
is the entire attack surface, and it gets its own story.**
**Build** one guarded write path that every importer must use. Filenames sanitised and
constrained to the issue's own folder — reject `..`, absolute paths, symlinks, control
characters, reserved Windows names, and names over 255 bytes. Per-file and per-issue size
caps. A decompressed-size limit for anything archived. Content type sniffed rather than taken
from the server's header. API tokens read from the environment or a credential helper, never
from a flag, never written to disk, and redacted from every log line and error string.
**Tests first** one payload per attack: `../../etc/passwd`, an absolute path, a symlink, a
1 GB attachment, a zip bomb, a 4,000-character filename, a filename containing a newline, and
a token interpolated into an error message.
**Done when** every importer writes exclusively through this path, asserted by a grep test in
the same style as M2-S1.

### M7-S3 · Resolving-commit recovery
**Branch** `isu/M7-S3-evidence-scan`
**Build** one pass over history recovering issue-key → commit links at three tiers: key in a
commit message, key in a merge commit's branch name, key in a squash subject. (Issues resolved
after the switch to isu carry an `Isu-Resolves:` trailer and need none of this — these tiers
exist for the years of history that predate it.) Record which
tier produced each link; unlinked issues import with their resolution date only.
**Tests first** a fixture repo deliberately mixing all three conventions plus a long tail of
commits with no key; assert per-tier counts exactly; assert an issue matched at two tiers
records the stronger one.
**Done when** the scanner runs against a real repository and reports its coverage.

### M7-S4 · Jira: issues, types and hierarchy
**Branch** `isu/M7-S4-jira-core`
**Build** read a Jira JSON export. Map issue types to the **five** isu types — a Jira Epic
becomes `type: epic` and, like every epic, is written **without `state:`**; everything else maps
to `bug`, `story`, `chore` or `spike`. Map status to state for those: terminal statuses become
`resolved` or `dropped`, and **everything non-terminal becomes `open`** — in-flight statuses are
not imported, because in this model they are derived from branches, and a ticket parked in
review for eight months was never in review. A `dropped` issue also needs `resolution`, which is
now mandatory: map Jira's own resolution field onto the enum, and anything unrecognised becomes
`wontfix` with the original string preserved in `source.yml`. Collapse Epic Link, parent and
subtask into the single `parent` field — and since M5-S2 requires a `parent` to name an epic,
a subtask whose parent is an ordinary issue keeps the link only when that parent is itself
imported as an epic; otherwise the link goes to `source.yml` and is reported in the dry run.
**Tests first** a fixture export covering epics, subtasks, dropped issues and a four-level
hierarchy; assert the collapse is lossless in the sense that the parent graph is preserved and
acyclic; assert imported Epics carry `type: epic` and no `state:`; assert every dropped issue
has a `resolution` drawn from the enum; assert a subtask under a non-epic parent does not emit
a `parent` that would fail the check.
**Done when** the imported tree passes `isu check` with zero failures — which, with the epic
and resolution rules above, is now a reachable bar rather than a contradiction.

### M7-S5 · Jira: comments, attachments and dev-status links
**Branch** `isu/M7-S5-jira-content`
**Build** comments become files under `comments/`, named by date, author and sequence per
section 1 — an active Jira ticket routinely has three comments from the same person on the same
day, and without the sequence the importer would silently keep only the last. Attachments
download through the M7-S2 write path subject to the caps. Optional `--dev-status` queries
Jira for the commit and branch links it already stores — for a team that installed the
Jira/forge integration, this is the highest-yield evidence source there is.
**Tests first** against a recorded transcript, never the live API: a 40 MB attachment is
skipped with a warning rather than crashing; a comment containing frontmatter delimiters does
not corrupt the issue file; `--dev-status` results outrank the M7-S3 scan when both have a
link for the same issue.
**Done when** a realistic export imports completely and idempotently — running it twice
produces a zero-length diff.

---

# M8 · Public website

The site is designed and built from scratch in this milestone. There is no approved comp to
port — treat M8-S1 as real design work with a written brief, not as implementation.

The governing constraint: **every sample shown on the site is generated by running this
repo's binary during the build.** No hand-written terminal HTML, no invented output. If the
product changes and the site doesn't, the build fails.

Three stories, not seven. The site sells v1.0.0; it does not gate it, and every week spent here
is a week the thing it advertises is not shipping.

### M8-S1 · Content plan and information architecture
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

### M8-S2 · Landing page
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

### M8-S3 · Docs, gates and deploy
**Branch** `isu/M8-S3-docs-deploy`
**Build** `docs/`: getting started, the data model, every derived status with its rule, the
check catalogue, the JSON contract, importing from Jira, and a page on what isu deliberately
does not do. Then `make site` producing the whole site from a clean checkout, the gates —
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
- **Linear import.** Jira is where the teams looking for an exit actually are. One importer,
  done properly, beats two done at the same time.
- **The WebAssembly derivation demo.** The most convincing possible proof of the central claim,
  and pure marketing: it gates nothing and it is the hardest thing on the website.
- **isu Tower** — the hosted app for people without a clone. Separate repo, after v1.
- Cross-repo issues. Monorepo-first is a position, not an omission.
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

1. The failing test is its own commit, and it precedes the commit that makes it pass.
2. `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./...` all pass at the
   branch head.
3. Coverage is at or above 85% overall, 100% for `internal/model`.
4. The story's own issue file is flipped to `resolved` in the same pull request (from M5-S7,
   which is where issue files start existing).
5. The pull request describes what changed, what was decided, and anything that needs a call.
6. The same pull request marks the story done in this file — heading ✅, `**Done**` line,
   milestone status column, per §0.
7. You have not merged it.
