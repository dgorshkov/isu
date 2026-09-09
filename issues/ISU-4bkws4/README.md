---
schema: 1
id: ISU-4bkws4
title: M1-S1 · Frontmatter parser
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-7076wc
blocked_by: ISU-3ymsy4
acceptance: the round-trip test passes on every fixture in `testdata/issues/`.
---
**Done** #5, 2026-08-26. Blank lines inside the block are preserved and comments are not part
of the format — this is not YAML, so a line with no colon is a parse error. `Document.Set`
flattens line breaks in a value to spaces, because the format has no folding and a value
carrying one would write a file that does not parse back.
**Branch** `isu/M1-S1-frontmatter`
**Build** `internal/issue`: parse `---` delimited key/value frontmatter plus body. Unknown
keys are preserved verbatim on round-trip. Parsing never panics on malformed input; it
returns a typed error with line number.
**Tests first** table-driven: valid, missing close delimiter, duplicate key, empty file, CRLF
line endings, unicode values, a 1 MB body. Plus a round-trip property test: parse → serialise
→ parse yields an identical struct.
**Done when** the round-trip test passes on every fixture in `testdata/issues/`.
