---
schema: 1
id: ISU-j49hvk
title: M9-S1 · Cross-platform build
type: story
state: open
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-thyf24
blocked_by: ISU-dj6g6k
acceptance: `goreleaser release --snapshot` produces working binaries, and a bad stamp fails
---
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
