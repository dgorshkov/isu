---
schema: 1
id: ISU-2ws116
title: M7-S1 · Import framework and dry run
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-2wyps1
blocked_by: ISU-ahaqb3
acceptance: `--dry-run` is the default and writing requires `--write`.
---
**Done** #15, 2026-09-03. The seam is four methods and three of them are description, because
everything after "which tracker" — ids, folders, the dry run, the guarded write — is the same for
every source there will ever be. A second tracker is a `Load` method, not a second importer.

**The reverse mapping is recorded rather than computed, and that is not laziness.** This story
asks that an id map back to the source key it was formed from, for a key that was legal and for
one that was not — and with a prefix of `PROJ`, the key `#1234` and the key `PROJ-1234` form the
same id. No function of the id alone can say which one it came from, so a `Mapping` remembers.
That also makes a collision between two keys something the type refuses rather than something the
filesystem resolves by overwriting.

**Mapping is two passes, not one, and the second one is a correction this document did not
anticipate.** A link is only good once the whole import is known: an issue whose epic was refused
for want of an owner would otherwise be written with a `parent` naming a folder nothing will
write, which is precisely what M5-S2 fails a repository for. So the issues are built, the links
that point at anything unwritten are pruned into `source.yml`, and only then are the files
rendered. `Dangling` in the dry run is that number.

`source.yml` is sorted at every level rather than left to a map's iteration order, which is what
makes M7-S5's idempotency assertion possible at all: a file whose key order changes per run
produces a diff every run.

**The importer's documentation executes now, and collecting that debt found a defect.** M8-S3
landed first and handed this pull request one specific thing to collect: give `docs/importing.md`
`console` blocks running `isu import github` against a recorded dump, so the importer's page runs
like every other page on the site. The first attempt failed, and the reason is worth writing
down. `--dump` read its path with `os.ReadFile`, while every other path in this product resolves
against `cli.Env.Dir` — the directory isu was run in, carried rather than read from the process
for the same reason `Getenv` and `Now` are carried. For `cmd/isu` the two are the same and
nothing was visibly wrong. For the site build, which runs the documentation's own commands in
process against a fixture repository somewhere else entirely, they are not. Both file paths the
command takes — `--dump` and `--fetch-only` — now resolve against that directory.

`internal/site/testdata/import/acme.json` is the recording the page imports: six rows of the REST
list, one of them a pull request, covering the three issue types, all three state reasons, a
milestone, two comments from one person on one day, a dependency inside the import and one
outside it, an attachment link, an issue field value, an unassigned issue and a closing pull
request reference. It is a file rather than a request because a page that needs github.com to
build is a page that stops building. `docs/importing.md` is no longer the only page under `docs/`
with no `$ isu` line in it.
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
