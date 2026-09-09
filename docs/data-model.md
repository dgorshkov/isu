# The data model

An issue is a folder. Everything else follows from that, and everything below is fixed for
schema version 1.

## The folder

```
issues/
  APP-7f3akq/
    README.md                    the issue
    repro.har                    attachments live with the issue
    comments/
      2026-08-24-support-01.md   append-only, one file per comment
```

`README.md` so that a forge renders the issue when you browse to the folder. Comment files are
`<date>-<author>-<nn>.md`, and the two-digit sequence is not decoration: without it, a second
comment by the same author on the same day silently overwrites the first.

Resolved issues stay in `issues/`. There is no `fixed/` directory and no archive command:
state lives in the file, so moving the file would be storing the state twice.

## The frontmatter

```
---
schema: 1              required, integer, the on-disk format version
id: APP-7f3akq         required, must equal the folder name
title: Login retries   required, one line
type: bug              required: bug | story | chore | spike | epic
state: open            required except on epics: open | resolved | dropped
owner: dana            required: the human answerable for it
created: 2026-02-21    required, RFC 3339 date
priority: p1           optional: p0 | p1 | p2 | p3, default p2
parent: APP-40b1cc     optional, an id; must name an epic
blocked_by: APP-39ka2p optional, comma-separated
repro: ...             required when type: bug
acceptance: ...        required when type: story
question: ...          required when type: spike
reason: ...            required when state: dropped
resolution: ...        required when state: dropped
---

Free-form markdown body.
```

Frontmatter is parsed by hand rather than by a YAML library, because writing an issue back has
to round-trip unknown keys, spacing and line endings byte for byte. A field you did not touch
is not rewritten, so an unmodified issue is a zero-length diff.

`schema` exists so that version 1 is not a format prison. A reader refuses a version it does
not know, naming the version it needs, rather than misparsing it. Adding optional keys does not
bump it; changing the meaning of an existing key does.

`title` is what the board, the list and every filter render. `created` is what issue age is
computed from — deriving it by walking history would cost a git process per issue. `priority`
is what `isu ready` orders by.

`owner` is the accountable human. It is set at triage and an agent must never change it; a
commit by a configured agent that reassigns an owner is a check failure. Who is *working* on an
issue right now is a different question, answered by the claiming branch and not by this field.

`resolution` is one of `wontfix`, `duplicate`, `works-as-intended` or `fixed-elsewhere`, and
sits alongside the free-text `reason`. `dropped` alone cannot distinguish a duplicate from a
won't-fix, and the difference is the first thing anyone asks.

## What each type requires

| type | required field | resolving it requires |
|---|---|---|
| `bug` | `repro` | a change outside `issues/` |
| `story` | `acceptance` | a change outside `issues/` |
| `chore` | — | a change outside `issues/` |
| `spike` | `question` | a file in the issue's own folder that isn't `README.md` |
| `epic` | — | nothing — an epic has no `state:`, and its status is the fold over its children |
| any | — | `state: dropped` requires `reason` and `resolution` |

An epic **must not** declare `state:`, and everything else **must**. Epic-ness is declared in
the file rather than inferred from who points at it, so validating one issue never requires
loading another — and no pull request of yours can invalidate a file somebody else owns by
adding a `parent:` line to a third file.

`parent` must name an issue of `type: epic`, and that is a repository-level rule rather than a
field-level one. Validating one issue checks that `parent` is *shaped* like an id and stops
there; whether it resolves, and whether what it resolves to is an epic, is a
[check](checks.html) with the whole set loaded.

## Ids

`<PREFIX>-<token>`, with the prefix from `.isu.yml`. The token is the first thirty bits of
`sha256(title ‖ owner ‖ created ‖ 8 bytes from crypto/rand)`, rendered as six characters of
Crockford base32 — lowercase, with `i`, `l`, `o` and `u` excluded so nothing is ambiguous read
aloud or typed from a screenshot.

The separator is a NUL byte and not concatenation. Running the fields together would make the
boundary between two of them imaginary, so a title ending in the owner's name would hash the
same as a shorter title and a longer owner.

Allocation needs no coordination, no counter and no network round trip, which is the whole
point: sequential `max + 1` cannot see issues sitting on unmerged `report/*` branches, so under
any real load it hands out the same number twice. `isu new` regenerates on a collision with any
id it can see — trunk plus every local ref.

**Ids are permanent and are never rewritten.** There is no `isu renumber`. That is what lets
`blocked_by` on one branch keep pointing at the right issue while a hundred other branches are
in flight, and it is why an imported issue keeps its source key verbatim: `PROJ-1234` is a
perfectly legal id, and the format above governs generation rather than validation.

## Configuration

`.isu.yml` sits at the repository root. This is the whole schema, and an unknown key is a
validation error rather than a silent no-op.

| key | type | default | meaning |
|---|---|---|---|
| `prefix` | string | — | required; the issue id prefix, and a legal folder name |
| `agents` | list of strings | empty | commit authors treated as agents by the owner rule |
| `direct_triage` | bool | `false` | allow `isu triage --push` to write straight to trunk |
| `stale_days` | int | `7` | a claim older than this is stale |
| `fetch_warn_hours` | int | `24` | warn when the newest remote ref is older than this |
| `attachment_max_bytes` | int | `524288` | per-attachment cap enforced by the attachments rule |

`isu init` writes `prefix` and nothing else.

## Trunk is where state is true

A branch saying `state: resolved` is a *proposal*. It becomes a fact about the repository when
the branch merges, and until then the only thing that reads it is a board that renders it as
`in progress`. This is not a special case for claims — it is the whole of what a claim is.

`--ref` chooses what isu treats as trunk, and defaults to `origin/HEAD`, then `main`, then
`master`, then `HEAD`. Never `HEAD` first: half of what isu says is how a branch differs from
trunk, and a repository read from its own checkout has nothing to differ from.

Post-merge questions are answered from **file content at trunk commits**, never from commit
metadata, because a squash merge collapses authorship and does not touch the file. The one
exception is linking a trunk commit back to the issue it resolved, which file content cannot
answer: `isu resolve` writes an `Isu-Resolves: <id>` commit trailer for exactly that.
