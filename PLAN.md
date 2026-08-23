# isu v1.0.0 — build plan

`isu` is an issue tracker with no database. Issues are folders inside the repo; a pull
request that fixes a bug also closes it, in the same diff. Written in Go, shipped as one
binary containing a CLI, a TUI and a local web UI.

This document is the build order. Work it top to bottom, one story per branch, one pull
request per story. Do not skip ahead, do not batch stories, do not merge your own work.

---

## 0. Working agreement

**Read this section before every session.**

- **One story = one branch = one pull request.** Branch name is the story id, slugged:
  `isu/M3-S2-derive-epic-rollup`.
- **TDD, visibly.** The first commit on every branch contains a failing test and nothing
  else. The second commit makes it pass. Refactor after. A reviewer must be able to see
  red-then-green in the commit history.
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
  `golangci-lint run`, `go test ./...` all pass, and coverage does not drop.

### Stack

| | |
|---|---|
| Language | Go 1.23+ |
| CLI | `spf13/cobra` |
| TUI | `charmbracelet/bubbletea`, `bubbles`, `lipgloss` |
| TUI tests | `charmbracelet/x/exp/teatest` |
| Assertions | `stretchr/testify/require` |
| Web | stdlib `net/http`, `html/template`, `embed`. No JS framework, no build step. |
| Release | `goreleaser` |

That is the entire dependency allowlist. Adding anything else requires asking first.

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

`ref:path` costs a tree walk per lookup. Object ids skip it. M2-S3 enforces this with a
benchmark that fails the build if it regresses.

---

## 1. The data model

Decided. Do not deviate without asking.

### Layout

```
issues/
  AR-41/
    README.md                 the issue
    repro.har                 attachments live with the issue
    comments/
      2026-08-24-support.md   append-only, one file per comment
```

`README.md` so that forges render the issue when you browse to the folder.

**Resolved issues stay in `issues/`.** There is no `fixed/` directory and no archive command
in v1. State lives in the file.

### Frontmatter

```yaml
---
schema: 1              required, integer, the on-disk format version
id: AR-41              required, must equal the folder name
type: bug              required: bug | story | chore | spike
state: open            required: open | resolved | dropped
owner: dmitry          required: the human answerable for it
parent: AR-40          optional
blocked_by: AR-39      optional, comma-separated
repro: ...             required when type: bug
acceptance: ...        required when type: story
question: ...          required when type: spike
reason: ...            required when state: dropped
---

Free-form markdown body.
```

`schema` exists so that v1.0.0 is not a format prison. A reader must refuse a `schema:` it
does not know, with a message naming the version it needs, rather than misparsing it. Adding
optional keys does not bump it; changing the meaning of an existing key does.

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
| any | — | `state: dropped` requires `reason` and **no** change outside `issues/` |

### Derived statuses

Never stored. Computed from `state` read across refs.

| status | rule |
|---|---|
| `awaiting triage` | folder exists on a branch, not on trunk |
| `open` | on trunk with `state: open`, no branch claims it |
| `in progress` | some branch has `state: resolved` where trunk has `open` |
| `done` | trunk has `state: resolved` |
| `dropped` | trunk has `state: dropped` |
| `reopened` | trunk has `state: open`, and some earlier trunk commit had `resolved` |

Annotations on `in progress`: **claimant** (author of the commit that flipped the state),
**age** (branch tip date), **stale** (tip older than 7 days), **contended** (more than one
branch claims it).

An issue that other issues name as `parent` is an **epic**. Its state is a fold over its
children and it must not declare `state:` of its own. All children terminal → resolved,
unless all are dropped → dropped. Otherwise open.

### IDs

`<PREFIX>-<n>`, prefix from `.isu.yml`, `n` allocated as max-existing + 1 at creation time.
Concurrent allocation will occasionally collide; that is fine, because a duplicate id is a
hard CI failure and every new issue arrives through a pull request. `isu renumber` fixes a
rejected one. **Never renumber on import.**

### Claims

`isu claim AR-41` does, in this order:

1. `git push origin <sha>:refs/claims/AR-41` — atomic compare-and-swap. If the ref exists and
   is not an ancestor, the push is rejected and the claim fails. **Do this first**, before
   any work, so the loser wastes nothing.
2. create branch `isu/AR-41`, flip `state: resolved`, set nothing else
3. push the branch

`isu unclaim` deletes the claim ref. Claims are advisory for humans and binding for agents.

### Squash-merge safety

Post-merge questions are answered from **file content at trunk commits**, never from commit
metadata. Squash collapses authorship; it does not touch the file. Every derivation that
reads history must read blob content, not `%an`. M3-S5 tests a squash-only lifecycle.

---

## Milestones

Eleven milestones. Stop for review at the end of each.

| | milestone | ships |
|---|---|---|
| M0 | Foundations | repo, CI, lint, test harness |
| M1 | Issue files | parse, serialise, validate |
| M2 | Git layer | fast load from any ref and from the working tree |
| M3 | Derivation | statuses, epics, claims, contention |
| M4 | CLI | board, show, ready, new, claim, resolve, drop, field notes |
| M5 | Checks | `isu check`, hooks, GitHub Actions, GitLab CI, dogfooding |
| M6 | TUI | `isu ui` |
| M7 | Local web | `isu serve` |
| M8 | Importers | safe writes, Jira, Linear |
| M9 | Public website | content, design, landing page, live demo, docs, deploy |
| M10 | Release | goreleaser, brew, docs, v1.0.0 |

### Sizing

Estimated in pull requests, because the reviewer is the constraint and the compiler is not.

| milestone | PRs | shape |
|---|---|---|
| M0 Foundations | 4 | small, mostly config; M0-S4 is the one that matters |
| M1 Issue files | 4 | small, pure functions, heavy table tests |
| M2 Git layer | 4 | medium; M2-S2 is the trickiest parsing in the project |
| M3 Derivation | 5 | medium; this is the product, expect the most review time here |
| M4 CLI | 6 | medium; M4-S6 has no code and may generate several follow-ups |
| M5 Checks | 7 | small each, and highly parallel in principle |
| M6 TUI | 5 | medium; golden-frame tests are fiddly to stabilise |
| M7 Local web | 2 | small, but M7-S2's security tests are non-negotiable |
| M8 Importers | 6 | large; M8-S5 and M8-S6 are the biggest single PRs in the plan |
| M9 Website | 7 | medium; M9-S4 is genuinely hard |
| M10 Release | 4 | small, except M10-S3 which is open-ended by design |

**54 pull requests.** At three reviewed per day that is roughly four weeks; at one per day,
roughly eleven. The agent is not the bottleneck — plan your own calendar, not its.

Two milestones can generate unplanned work and should not be scheduled tightly: **M4-S6**,
where real repositories get their say, and **M10-S3**, where hardening turns every defect into
a regression test first.

---

# M0 · Foundations

### M0-S1 · Repository skeleton
**Branch** `isu/M0-S1-skeleton`
**Decide first** the module path. `github.com/dgorshkov/isu` is a placeholder — confirm the
real one before the first commit, because changing it later rewrites every import in the tree.
**Build** `go.mod`, `cmd/isu/main.go` printing version, Apache-2.0
`LICENSE`, `README.md` with one paragraph and the install line, `.gitignore`, `.isu.yml`
carrying `prefix: ISU`.
**Tests first** `TestVersionCommand` asserts `isu --version` prints a semver string.
**Done when** `go run ./cmd/isu --version` works and the tree is green.

### M0-S2 · Lint, vet, coverage gate
**Branch** `isu/M0-S2-quality-gates`
**Build** `.golangci.yml` (errcheck, govet, staticcheck, revive, gofumpt), a `Makefile` with
`make test lint cover`, and a coverage script that fails under **85%** for `./internal/...`.
**Tests first** a test that shells the coverage script against a fixture below threshold and
asserts non-zero exit.
**Done when** `make lint` and `make cover` both pass locally.

### M0-S3 · CI on both forges
**Branch** `isu/M0-S3-ci`
**Build** `.github/workflows/ci.yml` and `.gitlab-ci.yml`. Both run build, vet, lint, test,
coverage on Linux and macOS. Both call the same `make` targets — no logic in YAML.
**Tests first** `TestMakefileTargetsExist` parses the Makefile and asserts every target the
CI files reference is defined. This is what keeps the two forges honest.
**Done when** both pipelines are green.

### M0-S4 · The git test harness
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
**Tests first** one case per row of the type table, plus: id not matching folder, unknown
type, unknown state, dropped without reason, bug without repro.
**Done when** `Validate()` output is stable, sorted and human-readable.

### M1-S3 · Reading and writing an issue folder
**Branch** `isu/M1-S3-folder-io`
**Build** load an issue from `issues/<ID>/`, listing attachments and `comments/`. Write an
issue back, preserving unknown keys and body byte-for-byte where unchanged.
**Tests first** a folder with attachments and three comments loads with all of them; writing
an unmodified issue produces a zero-length diff (assert with `git diff --exit-code`).
**Done when** the zero-diff test passes. This property matters more than it looks: it is what
keeps pull requests readable.

### M1-S4 · ID allocation and `.isu.yml`
**Branch** `isu/M1-S4-ids`
**Build** config loading, prefix validation, `NextID()` = max existing + 1, `isu renumber`.
**Tests first** allocation over an empty repo, a repo with gaps, a repo with 5,000 issues
(assert allocation is not O(n) reads); renumber updates the folder, the `id:` field and every
`parent`/`blocked_by` reference pointing at it.
**Done when** renumbering an issue leaves no dangling references.

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
**Why** `isu ui` and `isu serve` read what is on disk, uncommitted edits included. That is a
different path from `LoadRef`, and a faster one — no git process at all. M6 and M7 both depend
on it, so it is built here rather than discovered there.
**Build** `repo.LoadWorktree(root)` walking `issues/` and returning the same map type as
`LoadRef`. Honours `.gitignore`. An issue that parses but does not validate is reported, not
fatal — a half-written issue must not blind the whole board.
**Tests first** an uncommitted new issue and an uncommitted state flip both appear; a
malformed issue is reported rather than fatal; and a property test asserting `LoadWorktree`
and `LoadRef` agree exactly on a clean checkout of a generated repo.
**Done when** the agreement property holds on a 5,000-issue fixture.

### M2-S4 · The performance gate
**Branch** `isu/M2-S4-perf-gate`
**Build** a generator producing an N-issue fixture repo, and `BenchmarkLoadRef`.
**Tests first** `TestLoadRefUnder5000IssuesIsFast` builds 5,000 issues and **fails if
`LoadRef` exceeds 1.5 s**. Also assert that the number of `git` processes spawned is exactly
2, regardless of issue count — that is the assertion that actually prevents the regression,
because the slow paths differ by process count, not by algorithm.
**Done when** both tests pass in CI on the slowest runner.

# M3 · Derivation

### M3-S1 · Status derivation
**Branch** `isu/M3-S1-status`
**Build** `internal/model`: given trunk plus every branch, derive the six statuses from the
table in section 1. Pure function over loaded refs — no git calls inside.
**Tests first** one test per status, then the transitions between them. Include: issue on two
branches, issue on a branch identical to trunk, branch deleted after merge.
**Done when** the status function has 100% branch coverage. This package is the product; it
gets a higher bar than the rest.

### M3-S2 · Epic rollup
**Branch** `isu/M3-S2-epics`
**Build** children indexed once per load, not scanned per parent. Fold child states into the
epic. Cycle-safe: a parent cycle must return a value, never recurse forever.
**Tests first** rollup with mixed children; all dropped; nested epics three deep; **an issue
that is its own parent**; a two-node parent cycle. The self-parent case blew a stack during
prototyping — write it before the implementation.
**Done when** a 5,000-issue fixture rolls up 100 epics in under 50 ms.

### M3-S3 · Claimant, age, staleness, contention
**Branch** `isu/M3-S3-claims`
**Build** for each branch claiming an issue, find the commit that flipped `state` to
`resolved`; its author is the claimant and its date the claim time. Branch tip date gives
staleness. Two or more claiming branches means contended.
**Tests first** single claim; two claims (assert both branches named); a claim backdated 11
days reads as stale; a branch with no per-file commit falls back to the `owner` field.
**Done when** contention and staleness come out of the same single scan.

### M3-S4 · Reopen detection
**Branch** `isu/M3-S4-reopen`
**Build** walk trunk's history for the issue file; if any earlier trunk commit had
`state: resolved` and trunk now has `open`, the status is `reopened`. Read blob content at
each commit — never commit metadata.
**Tests first** merge then revert reads as reopened; merge, revert, re-fix reads as done;
never-resolved reads as open; **and the same three with the branch deleted after merge**.
**Done when** reopen is correct with zero branches left in the repo.

### M3-S5 · Squash-merge lifecycle
**Branch** `isu/M3-S5-squash`
**Build** nothing new. This story exists to prove the model survives the merge strategy most
teams use.
**Tests first** an end-to-end lifecycle using **only** squash merges: report → triage →
claim → fix → merge → revert, asserting the derived status at every step. Then assert the PR
number is recoverable from the trunk commit subject that set `resolved`.
**Done when** the squash lifecycle test passes without changing M3-S1..S4.

---

# M4 · CLI

### M4-S1 · Command scaffold and output contract
**Branch** `isu/M4-S1-cli-scaffold`
**Build** cobra root, `--repo`, `--ref`, `--json`, `--no-color`. **Every command supports
`--json`**, because half the users are agents. JSON shape is a stable contract: document it in
`docs/json.md` in this story.
**Tests first** golden-file tests for help output; a test asserting every registered command
accepts `--json`; a test asserting JSON output is valid and matches the documented schema.
**Done when** adding a command without `--json` fails the test suite.

### M4-S2 · `isu board` and `isu show`
**Branch** `isu/M4-S2-board-show`
**Build** the derived board grouped by status, and single-issue detail including attachments,
comments, claim and epic position.
**Tests first** golden output for a repo in every status; `--json` round-trips through
`encoding/json` into the documented struct.
**Done when** `isu board` on the isu repo itself renders this plan's milestones.

### M4-S3 · `isu new` and `isu ready`
**Branch** `isu/M4-S3-new-ready`
**Build** `new` scaffolds a folder, allocates an id, creates a `report/<ID>` branch and stages
it for a pull request. `ready` lists open issues whose `blocked_by` are all terminal, ordered,
`--json` by default for agents.
**Tests first** `new` produces a valid issue and a branch not on trunk (status: awaiting
triage); `ready` excludes blocked, dropped, claimed and contended issues.
**Done when** `isu ready --json | head -1` gives an agent everything it needs to start.

### M4-S4 · `isu claim` and `isu unclaim`
**Branch** `isu/M4-S4-claim`
**Build** the three-step claim from section 1, in that order. Claim failure must be fast,
quiet and exit non-zero with a machine-readable reason.
**Tests first** the rejection path **deterministically**: create the claim ref, then claim
from a second clone and assert the failure names the holder and exits non-zero. Repeating a
network operation a hundred times per CI run buys confidence in the network, not the code — so
the hundred-run stress variant lives behind `//go:build stress` and is out of the default
suite. Also: unclaim releases; claiming an already-terminal issue is refused; **claiming with
the remote detached fails without leaving a local branch behind**.
**Done when** the deterministic rejection test passes and `make stress` exists for the rest.

### M4-S5 · `isu resolve` and `isu drop`
**Branch** `isu/M4-S5-resolve-drop`
**Build** flip state on the current branch, with `drop` requiring `--reason`. Neither command
merges anything; both leave a branch for a pull request.
**Tests first** resolve on a spike without an artifact warns; drop without reason is refused;
both refuse to run directly on trunk.
**Done when** the only way to reach `done` is a merged pull request.

### M4-S6 · First contact with a real repository
**Branch** `isu/M4-S6-field-notes`
**Why** Everything so far has run against fixtures written by the same person who wrote the
assumptions. This is the first story where the world gets a vote, and it is deliberately
before the TUI, the web UI and the importers are built on top of those assumptions.
**Build** run the CLI's read paths against three real repositories of different shapes: a
large monorepo, one with submodules, and one with a decade of history and heavy squash-merge
use. Read only — nothing is written and nothing is imported. Record what happened in
`docs/field-notes.md`, including the timings.
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
parent cycles, self-parent, dependency cycles, epic declaring its own state, attachment size
cap (default 512 KB, configurable).
**Tests first** one repo fixture per violation, and one clean fixture asserting zero findings.
**Done when** every rule in section 1 that can be checked without a diff is checked.

### M5-S3 · Evidence checks
**Branch** `isu/M5-S3-evidence-checks`
**Build** the type table's resolution rules: resolving requires a change outside `issues/`;
a spike requires an artifact in its own folder; a drop requires a reason and no code change.
Only applies to issues the branch actually changed.
**Tests first** resolved-with-no-code fails; resolved-with-code passes; spike with only
`README.md` fails; spike with `decision.md` passes; drop with code changes warns.
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
claims older than the stale threshold.
**Tests first** two branches claiming the same issue produces one warning naming the other
branch; stale claim produces a warning with the age in days.
**Done when** both surface at pull-request time rather than at merge.

### M5-S6 · Hooks and CI templates
**Branch** `isu/M5-S6-init`
**Build** `isu init` writing a pre-commit hook, `.github/workflows/isu.yml` and
`.gitlab-ci.yml`. Flags `--hooks`, `--actions`, `--gitlab`. Idempotent: running twice changes
nothing. Never overwrites an existing file without `--force`.
**Tests first** init into a clean repo produces working files; init twice produces a
zero-length diff; init over an existing workflow without `--force` refuses.
**Done when** both generated pipelines run `isu check` and fail the build correctly.

### M5-S7 · Dogfooding switch
**Branch** `isu/M5-S7-dogfood`
**Why** Last in this milestone on purpose. It needs `isu new` from M4-S3 to create the issues
and the whole check suite to keep them honest; converting any earlier means hand-maintaining
issue files with nothing verifying them.
**Build** convert every remaining story in this plan into an issue folder **using `isu new`**.
Milestones become epics with no `state:` of their own; `blocked_by` encodes the order. Turn
`isu check` on for this repository's own pipeline.
**Tests first** a test loading `issues/` from trunk asserting every issue validates, the
dependency graph is acyclic, and every epic has at least one child.
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
**Build** rendered body, claim line, acceptance/repro, attachments, comment list, epic
position, and `blocked_by` showing each blocker's own status.
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

# M7 · Local web

### M7-S1 · Server and read-only board
**Branch** `isu/M7-S1-serve`
**Build** `isu serve` on `127.0.0.1` by default, stdlib router, `embed`-ed templates and CSS,
no JS build step. Reads the working tree; watches for changes and refreshes.
**Tests first** `httptest` against a fixture repo: board renders every status; unknown route
404s; the server binds loopback only unless `--host` is given.
**Done when** `isu serve` renders the same board as `isu board`.

### M7-S2 · Issue pages and attachments
**Branch** `isu/M7-S2-issue-pages`
**Build** an issue page with rendered markdown, comment threads, and attachments served with
correct content types. Markdown rendering must sanitise — attachments are untrusted input.
**Tests first** an issue with an HTML attachment does not execute script; a comment containing
a `<script>` tag renders inert; path traversal in an attachment name is rejected.
**Done when** the XSS and traversal tests pass. Do not skip these.

---

# M8 · Importers

### M8-S1 · Import framework and dry run
**Branch** `isu/M8-S1-import-framework`
**Build** `internal/importer`: a source interface, field mapping to the isu schema, an
evidence-tier recorder, and a `--dry-run` report showing counts, coverage and samples without
writing anything. **Keys are never renumbered.** Unmapped source fields go to `source.yml` in
the issue folder, never into frontmatter — a mature tracker has two hundred custom fields and
they must not poison the schema.
**Tests first** dry run writes no files; mapping preserves keys; a source with two hundred
custom fields produces clean frontmatter and a complete `source.yml`.
**Done when** `--dry-run` is the default and writing requires `--write`.

### M8-S2 · Safe writes
**Branch** `isu/M8-S2-safe-writes`
**Why** An importer writes attacker-influenced data into your repository. Ticket titles,
filenames and attachments all originate outside your control. Serving that data is guarded in
M7-S2; **writing it is the more dangerous direction and gets its own story.**
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

### M8-S3 · Resolving-commit recovery
**Branch** `isu/M8-S3-evidence-scan`
**Build** one pass over history recovering issue-key → commit links at three tiers: key in a
commit message, key in a merge commit's branch name, key in a squash subject. Record which
tier produced each link; unlinked issues import with their resolution date only.
**Tests first** a fixture repo deliberately mixing all three conventions plus a long tail of
commits with no key; assert per-tier counts exactly; assert an issue matched at two tiers
records the stronger one.
**Done when** the scanner runs against a real repository and reports its coverage.

### M8-S4 · Jira: issues, types and hierarchy
**Branch** `isu/M8-S4-jira-core`
**Build** read a Jira JSON export. Map issue types to the four isu types. Map status to state:
terminal statuses become `resolved` or `dropped`, and **everything non-terminal becomes
`open`** — in-flight statuses are not imported, because in this model they are derived from
branches, and a ticket parked in review for eight months was never in review. Collapse Epic
Link, parent and subtask into the single `parent` field.
**Tests first** a fixture export covering epics, subtasks, dropped issues and a four-level
hierarchy; assert the collapse is lossless in the sense that the parent graph is preserved and
acyclic.
**Done when** the imported tree passes `isu check` with zero failures.

### M8-S5 · Jira: comments, attachments and dev-status links
**Branch** `isu/M8-S5-jira-content`
**Build** comments become files under `comments/`, named by date and author. Attachments
download through the M8-S2 write path subject to the caps. Optional `--dev-status` queries
Jira for the commit and branch links it already stores — for a team that installed the
Jira/forge integration, this is the highest-yield evidence source there is.
**Tests first** against a recorded transcript, never the live API: a 40 MB attachment is
skipped with a warning rather than crashing; a comment containing frontmatter delimiters does
not corrupt the issue file; `--dev-status` results outrank the M8-S3 scan when both have a
link for the same issue.
**Done when** a realistic export imports completely and idempotently — running it twice
produces a zero-length diff.

### M8-S6 · Linear import
**Branch** `isu/M8-S6-linear`
**Build** the same through Linear's GraphQL API: issues, projects as epics, comments,
attachments. Cycles are ignored deliberately. Paginate, back off on rate limits, and resume
from a cursor after interruption.
**Tests first** against a recorded transcript: pagination across three pages, a 429 followed
by successful backoff, and resume after an interruption mid-import leaving no partial issue
folder behind.
**Done when** both importers produce trees that differ only in their `source.yml`.

---

# M9 · Public website

The site is designed and built from scratch in this milestone. There is no approved comp to
port — treat M9-S1 and M9-S2 as real design work with a written brief, not as implementation.

The governing constraint: **every sample shown on the site is generated by running this
repo's binary during the build.** No hand-written terminal HTML, no invented output. If the
product changes and the site doesn't, the build fails.

### M9-S1 · Content plan and information architecture
**Branch** `isu/M9-S1-content-plan`
**Why** The page has one job: an engineer decides in thirty seconds whether this is a toy.
Decide what must be proved, and in what order, before anything is designed.
**Build** `web/CONTENT.md` — the section sequence, the single claim each section makes, the
artifact that proves it, and the finished copy. Plus the docs tree and the navigation. No
design, no markup, in this story.
**Tests first** every command and code sample in `CONTENT.md` executes against a scratch repo
and produces the output the document claims.
**Done when** someone who has never seen the project can read `CONTENT.md` and say in one
sentence what isu does and who it is for.

### M9-S2 · Design system
**Branch** `isu/M9-S2-design-system`
**Build** `web/DESIGN.md` recording the decisions: four to six named colours with hex, the
typefaces and their roles, the type scale, the layout concept, and the one signature element
the site will be remembered by. Then `web/assets/site.css` implementing those as custom
properties. Fonts are self-hosted and subset — no third-party font CDN.
**Tests first** a test asserting every colour and font-size in the templates resolves to a
declared custom property, and a test asserting the built HTML makes zero third-party network
requests.
**Done when** no template hardcodes a hex value or a pixel font size.

### M9-S3 · Landing page
**Branch** `isu/M9-S3-landing`
**Build** the page from `CONTENT.md` using the design system. Terminal output, board renders
and check results are produced by running `isu` against a fixture repo at build time and
embedded — never authored by hand.
**Tests first** a test regenerating every sample and failing if the committed page differs.
This is what stops the site drifting from the product.
**Done when** every artifact on the page came out of the binary in this repo.

### M9-S4 · Live derivation demo
**Branch** `isu/M9-S4-wasm-demo`
**Why** The whole claim is that status is computed rather than stored. The only convincing
proof is letting a visitor compute it. This is possible because M3-S1 made derivation a pure
function over loaded refs — collect on that.
**Build** compile `internal/model` to WebAssembly. A demo over a synthetic repo where the
visitor claims an issue, merges, reverts, and adds a branch, and the board re-derives using
the real engine. No reimplemented logic in JavaScript. Loads on interaction, not on page load.
**Tests first** a Go test running the same scenario list against the native build and against
the WASM build, asserting identical output; a size budget test failing over 2 MB gzipped.
**Done when** a reviewer can break the demo only by finding a real bug in `internal/model`.

### M9-S5 · Docs
**Branch** `isu/M9-S5-docs`
**Build** `docs/`: getting started, the data model, every derived status with its rule, the
check catalogue, the JSON contract, importing from Jira and Linear, and a page on what isu
deliberately does not do.
**Tests first** a test extracting every fenced shell block from the docs and running it
against a scratch repo. Documentation that does not execute is documentation that rots — and
these docs will be read by agents.
**Done when** every command in the docs actually runs.

### M9-S6 · Build, budgets and accessibility
**Branch** `isu/M9-S6-site-gates`
**Build** `make site` producing the whole site from a clean checkout. Then the gates: internal
link checker, HTML validity, a page-weight budget, an accessibility pass, responsive layout
down to 360 px, and `prefers-reduced-motion` honoured.
**Tests first** the budget test fails over 300 KB for any page excluding the demo payload; the
accessibility test fails on any violation; a viewport test asserts no horizontal scroll at
360 px.
**Done when** all gates run in CI on both forges.

### M9-S7 · Deploy and metadata
**Branch** `isu/M9-S7-deploy`
**Build** publish `web/` on merge to trunk from both pipelines. Favicon set, Open Graph and
Twitter cards, sitemap, canonical URLs, and a 404 page that is useful rather than decorative.
**Tests first** a workflow-lint test asserting the deploy job triggers only on trunk; a test
asserting every page has a title, a description and an OG image.
**Done when** the site is live and shareable links render a card.

# M10 · Release

### M10-S1 · Cross-platform build
**Branch** `isu/M10-S1-goreleaser`
**Build** goreleaser for linux/darwin on amd64 and arm64, static, reproducible, with version
and commit stamped in.
**Tests first** a smoke test running each built binary's `--version` under emulation where
available.
**Done when** `goreleaser release --snapshot` produces working binaries.

### M10-S2 · Distribution
**Branch** `isu/M10-S2-distribution`
**Build** homebrew tap formula, `go install` path verified, checksums and signatures.
**Tests first** a test installing from the built tarball into a temp prefix and running
`isu --version`.
**Done when** `brew install isu` works from a clean machine.

### M10-S3 · Release candidate hardening
**Branch** `isu/M10-S3-hardening`
**Build** no new features. Fix what the following surface: run `isu` against three real
repositories of different shapes; run the full lifecycle under a squash-merge-only repo; run
with the remote detached for the whole session; run with 20,000 issues.
**Tests first** convert every defect found into a regression test before fixing it.
**Done when** all four scenarios pass and coverage is at or above the gate.

### M10-S4 · v1.0.0
**Branch** `isu/M10-S4-v1`
**Build** CHANGELOG, README final pass, tag `v1.0.0`.
**Done when** the tag is pushed and the release artifacts are attached.

---

## Out of scope for v1.0.0

State this in the README so nobody has to ask:

- **isu Tower** — the hosted app for people without a clone. Separate repo, after v1.
- Cross-repo issues. Monorepo-first is a position, not an omission.
- A `fixed/` archive directory. Resolved issues stay in `issues/`.
- Sprints, story points, burndown, time tracking.
- Bidirectional sync with anything. Import is one-way and one-time by design.
- Notifications and email.

## Definition of done, every story

1. A failing test exists in the first commit.
2. `go build ./...`, `go vet ./...`, `golangci-lint run`, `go test ./...` all pass.
3. Coverage is at or above 85% overall, 100% for `internal/model`.
4. The story's own issue file is flipped to `resolved` in the same pull request (from M2-S4).
5. The pull request describes what changed, what was decided, and anything that needs a call.
6. You have not merged it.
