---
schema: 1
id: ISU-tdgkj4
title: M0-S3 · CI
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-3ymsy4
blocked_by: ISU-dagpgx
acceptance: the pipeline is green.
---
**Done** #4, 2026-08-25. **This story was amended as it was built.** It read *CI on both
forges* and shipped a `.gitlab-ci.yml` alongside the Actions workflow; the project lives on
GitHub only, so the GitLab pipeline was deleted in the same pull request and every other
reference to a second forge in this file was corrected with it — see `docs/not-doing.md`.
**Branch** `isu/M0-S3-ci`
**Build** `.github/workflows/ci.yml`, running build, vet, lint, test and coverage on Linux and
macOS. It calls `make` targets — no logic in YAML, so every gate can be run before pushing.
**Tests first** `TestMakefileTargetsExist` parses the Makefile and asserts every target the CI
file references is defined. This is what stops a gate existing only inside YAML.
**Done when** the pipeline is green.
