---
schema: 1
id: ISU-8za9zb
title: M9-S2 · Distribution
type: story
state: open
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-thyf24
blocked_by: ISU-j49hvk
acceptance: `brew install isu` works from a clean machine.
---
**Branch** `isu/M9-S2-distribution`
**Build** homebrew tap formula, `go install` path verified, checksums and signatures.
**Tests first** a test installing from the built tarball into a temp prefix and running
`isu --version`.
**Done when** `brew install isu` works from a clean machine.
