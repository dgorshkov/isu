---
schema: 1
id: ISU-dagpgx
title: M0-S2 · Lint, vet, coverage gate
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-3ymsy4
blocked_by: ISU-c9exxy
acceptance: `make lint` and `make cover` both pass locally, and neither floor can be met by a tree that violates the other.
---
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
the tree is not the gate the working agreement says it is.
**Tests first** a test that shells the coverage script against a fixture below the overall
threshold and asserts non-zero exit; a second fixture that clears 85% overall but leaves
`internal/model` under 100% and must also exit non-zero; a third asserting the per-package
floor is skipped, not failed, while `internal/model` does not yet exist.
**Done when** `make lint` and `make cover` both pass locally, and neither floor can be met by
a tree that violates the other.
