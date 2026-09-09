---
schema: 1
id: ISU-c9exxy
title: M0-S1 · Repository skeleton
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-3ymsy4
acceptance: `go run ./cmd/isu --version` works and the tree is green.
---
**Done** #1, 2026-08-24. Module path confirmed as `github.com/dgorshkov/isu`.
**Branch** `isu/M0-S1-skeleton`
**Decide first** the module path. `github.com/dgorshkov/isu` is a placeholder — confirm the
real one before the first commit, because changing it later rewrites every import in the tree.
**Build** `go.mod`, `cmd/isu/main.go` printing version, Apache-2.0
`LICENSE`, `README.md` with one paragraph and the install line, `.gitignore`, `.isu.yml`
carrying `prefix: ISU`.
**Tests first** `TestVersionCommand` asserts `isu --version` prints a semver string.
**Done when** `go run ./cmd/isu --version` works and the tree is green.
