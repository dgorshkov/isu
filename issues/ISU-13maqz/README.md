---
schema: 1
id: ISU-13maqz
title: M1-S3 · Reading and writing an issue folder
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-7076wc
blocked_by: ISU-w9m2z3
acceptance: the zero-diff test passes. This property matters more than it looks: it is what keeps pull requests readable.
---
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
