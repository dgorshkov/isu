---
schema: 1
id: ISU-hh9ry2
title: M4-S1 · Command scaffold and output contract
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-6mkqq5
blocked_by: ISU-stb9h3
acceptance: adding a command without `--json` fails the test suite.
---
**Done** #9, 2026-08-31. The two decisions this story had to make are recorded above: `isu init`
answers the data model's open question, and `--ref` no longer defaults to HEAD — it defaults to what
origin/HEAD says, then main, then master, then HEAD. Half of what this product says is how a
branch differs from trunk, so a trunk that follows you onto your feature branch answers every one
of those questions with "it does not", and leaves `isu resolve` unable to tell that it is about
to write straight onto trunk. **Output is newline-delimited JSON**, one complete value per line,
which is what makes M4-S3's `isu ready --json | head -1` the top of the queue rather than an
opening brace. The gate the story asks for is a table every registered command needs a row in;
beside it, a second test reflects over the payload structs and fails when a field exists in the
code and not in `docs/json.md`, so the contract and its document cannot drift by more than one
commit nobody ran the tests on.
**Branch** `isu/M4-S1-cli-scaffold`
**Build** cobra root, `--repo`, `--ref`, `--json`, `--no-color`, `--fetch`. **Every command
supports `--json`**, because half the users are agents. JSON shape is a stable contract:
document it in `docs/json.md` in this story.
`--fetch` updates remote refs before reading. Every derived status in this product is a
statement about refs, so a board computed from a week-old fetch is a board about last week —
and nothing else in the plan so much as mentions fetching.
**Tests first** golden-file tests for help output; a test asserting every registered command
accepts `--json`; a test asserting JSON output is valid and matches the documented schema; a
test asserting `--fetch` is a no-op against a repo with no remote rather than an error.

**The end-to-end harness ships with the first command, not after the last.** Noted in #8, where
measuring coverage found the honest gap: everything through M3 is tested end to end *through the
stack* — a real git repository, no mocks anywhere, M3-S5 walking a whole lifecycle — but nothing
is tested end to end *through the product*, because there is no product yet. `cmd/isu` is one
line delegating to a `run` that takes its streams as arguments, which is the shape that makes
this cheap; M4-S1 is where that stops being a convenience and becomes the thing every later
story's test is written against. A milestone that builds eight commands and writes its first
end-to-end test at M4-S8 has seven commands nobody ever ran.
**Done when** adding a command without `--json` fails the test suite.
