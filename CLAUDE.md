# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## PLAN.md governs

`PLAN.md` is the build order and the design record, not background reading. **Read §0 "Working
agreement" before every session**, and section 1 "The data model" before touching anything that
reads or writes an issue.

What it obliges you to, in short:

- **TDD, visibly.** The failing test is its own commit and precedes the commit that makes it pass.
  A Go test naming types that do not exist yet does not compile, and that is the intended red —
  CI gates the branch head, not every commit.
- **Commit format:** `M5-S3: require the work beside the word` — story id, colon, imperative
  summary. The body explains why, not what.
- **One pull request carries one story**, or several neighbouring stories in the same milestone.
  Never group across a milestone boundary. Never merge your own pull request.
- **Stop at every milestone boundary** and wait for explicit approval to start the next one.
- **A story is not finished until PLAN.md says so in the same pull request**: ` ✅` on the heading,
  a `**Done** #<pr>, <date>` line under it, the Milestones table row updated, and the milestone
  heading marked when its last story lands.
- **If a story needs a decision this document does not make, stop and ask** — put the question in
  the pull request description rather than inventing product behaviour.
- **If a test proves PLAN.md wrong, say so.** Open the pull request with the failing test, the
  evidence and a proposed amendment. A story that ends in a corrected PLAN.md and no
  implementation is a successful story.

Decisions are recorded where the decision lives: a milestone's header note and each story's
`**Done**` paragraph carry the corrections, defects and carve-outs that milestone produced. When
you change behaviour the plan specifies, write the reason there.

## Commands

Every gate is a make target, and CI calls these and nothing else.

```sh
make all       # build vet lint test perf cover dogfood — the whole gate
make build     # go build -o isu ./cmd/isu, and go build ./...
make lint      # golangci-lint (errcheck, govet, revive, staticcheck, gofumpt)
make fmt       # rewrite files to satisfy the formatters
make test      # go test ./...
make perf      # M2-S5's wall-clock budgets, measured with the machine to itself
make cover     # the coverage floors: 99% overall, 100% internal/model
make dogfood   # ./isu check over this repository, with the isu just built
make site      # rebuild web/site from web/CONTENT.md, docs/ and the binary
make stress    # the //go:build stress races, out of the default suite
make tools     # install the pinned golangci-lint
```

Narrower loops:

```sh
go test ./internal/check/ -run TestARingIsReportedOnce   # one test
go test ./internal/cli/ -run TestGoldenCheck -update     # rewrite golden files
go test ./internal/repo/ -run 'Fast|ProcessCount'        # the read-path gates
go test ./internal/repo/ -bench BenchmarkBoard -run '^$' # the read-path benchmarks
go test ./internal/site/ -run TestEveryCommandInTheDocs      # the docs, executed
```

**Coverage is a hard gate with about eight statements of headroom.** `scripts/coverage.sh`
enforces 99% across the whole module — `cmd/`, `scripts/` and the test harness included — and
100% for `internal/model`. An error return you add is an error return you must reach; the levers
that work here are a directory that is not a repository, a path something else is already sitting
on, and the git shim below.

## Architecture

One binary, layered so that every git process is spent in one place and every derivation is a
pure function over what that place loaded.

```
cmd/isu -> internal/cli -> internal/check   (rules, pure)
                        -> internal/model   (statuses, pure)
                        -> internal/repo    (every git process)
                        -> internal/gitx    (the only place that builds a git command)
           internal/issue  (the file format)   internal/config (.isu.yml)
           internal/gittest (real repositories, for tests)
           internal/site   (the website; calls internal/cli for its samples)
```

- **`internal/gitx` is the one door to git**, and `TestNothingOutsideGitxExecutesGit` enforces it
  by grepping `internal/` for `exec.Command`. `internal/cli/editor.go` is the single named
  exemption — it opens `$EDITOR` — and a second test asserts it never runs git. The harness goes
  through gitx too.
- **`internal/repo` owns the cost.** Loaders: `LoadRef`, `LoadBoard`, `LoadWorktree`,
  `LoadHistory`, `LoadFirstCommits`, `LoadBranch`, `LoadFiles`, `LoadWorktreeFiles`. If a
  derivation needs something new, the loader grows and the result is handed over.
- **`internal/model` derives and never calls git.** Statuses, the epic rollup, claims, reopens.
  It is a pure function of a `model.Input` and holds a 100% coverage floor.
- **`internal/check` is the same rule for the rules.** A `Check` is `Name/Scope/Describe/Run`
  over a `check.Input` the CLI assembled; it runs no git and never fails, because a check that
  cannot answer has found nothing.
- **`internal/issue` is the on-disk format.** Frontmatter is parsed by hand — not by a YAML
  library — because `Write` must round-trip unknown keys, spacing and line endings byte for byte.
  `Validate()` takes one issue and nothing else; anything needing a second issue is a check.
- **`internal/gittest` builds real repositories** in `t.TempDir()`. Nothing in this project is
  mocked: tests script history and let the code read it as it would read a clone. Its methods
  fail the test rather than returning an error (`Try` is the exception).

**The wall-clock budgets are asserted by `make perf` and nowhere else.** A budget on elapsed
time is a claim about the whole machine, and `go test ./...` runs `internal/cli`'s seventy
seconds of git beside the package being timed — which put this gate red on macOS three times
before the measurement was moved somewhere quiet. `make perf` runs the one package in one
process and sets `ISU_PERF` to say so, with `-v` so the number it measured is on the record of
every run; the default suite still runs those tests and logs the same figure, and the
**process-count** assertions beside them — the ones that actually prevent the regression — hold
in every pass. macOS carries a further factor-of-two allowance, because `macos-latest` is
measured at 2.58× the runner the budgets were taken on — 707 ms for `LoadRef` there against
274 ms here, and 3.66 s for the board against 1.41 s. Those left the plan's own numbers 2.12×
and 1.64× of headroom on an idle macOS runner, which is why both budgets carry it and not just
the one that went red. See PLAN.md M2-S5.

### The read path is a hard requirement

Loading every issue from a ref is `git ls-tree -r <ref>`, then **one** `git cat-file --batch` fed
**object ids, not `ref:path`** (0.6 s versus 4.9 s on 5,000 issues). The board is different again:
trunk is listed once and every other ref is *diffed* against it, so a branch costs its own
difference and not the repository. `internal/repo/perf_test.go` fails the build if either
regresses, and it asserts process *counts*, not just times.

### Domain rules worth knowing before you edit

- **Status is derived, never stored.** The precedence table in PLAN.md §1 is ordered and the first
  match wins; `reopened` survives as an annotation on whatever status beats it.
- **Trunk is where state is true.** A branch saying `state: resolved` is a proposal, and that
  proposal is exactly what a claim is: `isu claim` branches, flips the state, commits with an
  `Isu-Claim:` nonce trailer, and pushes — the push is the compare-and-swap.
- **`--ref` defaults to origin/HEAD, then main, then master, then HEAD.** Never HEAD first: half of
  what this product says is how a branch differs from trunk.
- **Post-merge questions are answered from file content at trunk commits, never commit metadata**,
  because squash collapses authorship and does not touch the file. The one exception is linking a
  commit to the issue it resolved, which is the `Isu-Resolves:` trailer.
- **Ids are permanent and never rewritten**, including imported ones (`PROJ-1234` is a valid id).

## Conventions this codebase enforces with tests

- **Every command supports `--json`.** `internal/cli/contract_test.go` fails when a registered
  command has no row in `jsonCases`, and `TestJSONDocumentsEveryField` fails when a payload field
  in `payload.go` is not documented in `docs/json.md`. Adding a command means: the command, a
  `jsonCases` row, a `docs/json.md` section, and a golden help file (`-update` writes it).
- **Adding a check is one file and one registry line** (`func init() { Checks.MustRegister(...) }`).
  The `isu check` help is generated from the registry, so the golden help file is the proof the
  rule is documented.
- **Golden files are the review.** `golden()` scrubs the only three things that cannot be stable —
  RFC 3339 moments, dates and object ids — and everything else in the file is deliberate.
- **Tests run on the real clock**, because gittest dates commits relative to it. Do not pin a test
  clock to a calendar date.
- **Error paths are reached, not hoped for.** `internal/cli/gitfails_test.go` puts a git on `PATH`
  that forwards to the real one and refuses exactly one invocation, counted (`refuseGit`,
  `refuseGitAfter`) — that is how "what does isu say when git will not answer" is asserted. These
  tests cannot run in parallel: `PATH` is process-wide.

## isu tracks its own construction

`issues/` holds one folder per remaining story — milestones are epics, stories name them as
`parent`, and `blocked_by` encodes the order. From M5-S7 on, **the pull request that carries a
story also flips that story's issue file to `resolved`** (with `isu resolve`, which writes the
trailer), and M5-S3 turns a pull request that contains nothing else into a failing check.

`internal/cli/dogfood_test.go` holds PLAN.md and `issues/` to each other in both directions — a
story heading with no issue, or an issue with no heading, fails the suite. `make dogfood` runs the
rules over this repository with the binary just built.

## The website is a golden file

`web/CONTENT.md` **is** the landing page — `internal/site` parses the section sequence, the
claim, the copy and the sample out of it, and `web/templates/pages.html.tmpl` carries structure
and not one word. `docs/*.md` are the documentation pages; `internal/site/build.go` names them,
and `TestTheDocumentsAndTheDocumentationAgree` fails when a file in `docs/` is not on that list.

- **Every sample is produced by running isu**, in process through `cli.Run` against a repository
  `internal/site/fixture.go` builds with plumbing and a fixed clock. No `exec.Command`, no
  built binary, nothing taken from the machine the build runs on.
- **A ```console fence is executed.** Its `$ ` lines are run and the lines beneath each one are
  compared with what isu printed. A `$ ` prompt outside such a fence fails the extraction.
  ```console exit=1 is how a document quotes a command that is meant to fail.
- **`make site` is `go test ./internal/site/ -update`.** It rewrites `web/site/`, which is
  committed; `make test` fails when the committed site and the regenerated one differ. Change a
  renderer and the failure hands you both versions.
- **The gates run inside `Build`**, so `make site` refuses to write a site that fails one:
  internal links, HTML validity, a 300 KB per-page budget, an accessibility pass, WCAG contrast
  computed from the CSS tokens, a width the page survives at, reduced motion, and no colour or
  pixel type size outside `:root`.
- **Netlify publishes the site**, from `netlify.toml` — `.github/workflows/site.yml` holds no
  write permission and publishes nothing. What it does is regenerate `web/site` on every pull
  request and fail when the committed bytes differ from the built ones.

## Dependencies

The allowlist is fixed in PLAN.md §0 — cobra, bubbletea/bubbles/lipgloss/glamour (M6), teatest,
goccy/go-yaml (`.isu.yml` and `source.yml` only, never frontmatter), testify, goreleaser. **Adding
anything else requires asking first.** Go 1.24+; `git` is a hard runtime requirement.
