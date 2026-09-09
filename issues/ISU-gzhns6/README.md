---
schema: 1
id: ISU-gzhns6
title: M1-S5 · Schema version and migration
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-7076wc
blocked_by: ISU-8t3xyb
acceptance: a `schema: 2` repository fails with an error a human can act on, proven by a test rather than by inspection. ---
---
**Done** #5, 2026-08-26. **Two lines below were corrected as this story was built**: the
error message cannot name "the version of `isu` that understands it", because that build does
not exist; and "byte-identical version 1 file" did not say identical to what. Gating runs in
both directions — an older `schema:` is refused as needing migration rather than read on a
guess.
**Branch** `isu/M1-S5-schema-version`
**Why** `schema:` is the promise that v1.0.0 is not a format prison, and an untested promise is
decoration. This story is what makes the field real, and it is cheap now and expensive after
people have repositories.
**Build** version gating in the reader, **in both directions**: a known `schema:` loads, a
newer one is refused, and an older one is refused as needing migration rather than read on a
guess. The message names the version it found and the version this build reads. **It cannot
name "the version of `isu` that understands it"** — that build does not exist yet and nothing
in this repository can know its number — so it says what to do instead: *upgrade to a build of
isu that reads version N*. Plus a `Migration` interface and the registry that dispatches on
version, with zero migrations registered.
**Tests first** `schema: 1` loads; `schema: 2` is refused and the error names both versions;
`schema:` missing is refused; `schema: banana` is refused with a parse error rather than a
panic; a registered no-op migration from a fixture at version 0 produces a version 1 file
**byte-identical to the fixture apart from the `schema:` line itself** — unknown keys, blank
lines inside the block, spacing and the body all untouched. The registry writes that line
after each migration returns, so a migration with nothing to do is an empty method body and
cannot forget to bump the version or bump it twice.
**Done when** a `schema: 2` repository fails with an error a human can act on, proven by a test
rather than by inspection.

---
