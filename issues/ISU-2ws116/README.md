---
schema: 1
id: ISU-2ws116
title: M7-S1 · Import framework and dry run
type: story
state: open
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-2wyps1
blocked_by: ISU-ahaqb3
acceptance: `--dry-run` is the default and writing requires `--write`.
---
**Branch** `isu/M7-S1-import-framework`
**Build** `internal/importer`: a source interface, field mapping to the isu schema, an
evidence-tier recorder, and a `--dry-run` report showing counts, coverage and samples without
writing anything. **A source key becomes the id verbatim where it is already a legal one, and
is formed from it where it is not** — `PROJ-1234` stays `PROJ-1234`, GitHub's `#1234` becomes
`<PREFIX>-1234`, and either way the thing your team has been writing in commit messages for
years is still legible in the id. The data model governs the shape; this story owns the rule that
the mapping is total and reversible. Unmapped source fields go to `source.yml` in the issue
folder, never into frontmatter — a mature tracker has two hundred custom fields and they must
not poison the schema.
**Tests first** dry run writes no files; an id maps back to the source key it was formed from,
for a key that was legal and for one that was not; a source with two hundred custom fields
produces clean frontmatter and a complete `source.yml`.
**Done when** `--dry-run` is the default and writing requires `--write`.
