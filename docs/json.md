# isu's JSON output

Every isu command supports `--json`, because half the people reading its output
are agents. **What follows is a contract**, not a rendering: fields are added,
never repurposed, and an agent written against this document keeps working.

`internal/cli/payload.go` defines these shapes and
`TestJSONDocumentsEveryField` fails the build when a field exists there and not
here, so the two cannot drift by more than one commit nobody ran the tests on.

## The line format

Output is **newline-delimited JSON**: one complete JSON value per line.

Most commands print exactly one line. `isu ready` prints one issue per line, in
the order an agent should take them, so that

```sh
isu ready --json | head -1
```

is the top of the queue and not an opening brace. `ready` prints JSON **by
default**; pass `--json=false` for the list a person reads.

Errors go to standard error, as one `Failure` object and nothing else — no usage
text, no warnings. Exit status is `0` for success, `1` for a command that could
not do what it was asked, and `2` for a command line that does not make sense.

## Failure

Printed on standard error by any command that fails while `--json` is set.

| field | type | meaning |
|---|---|---|
| `error` | string | what went wrong, in one sentence |

## Issue

One issue: the fields its file carries, and what the repository derives about
them. Every field is always present — a list that is sometimes `null` makes
every consumer write the same defensive branch.

| field | type | meaning |
|---|---|---|
| `id` | string | the folder name, which is what every `parent` and `blocked_by` points at |
| `title` | string | one line |
| `type` | string | `bug`, `story`, `chore`, `spike` or `epic` |
| `state` | string | what the file says: `open`, `resolved`, `dropped`, or empty on an epic |
| `status` | string | what the repository says — see below |
| `owner` | string | the human answerable for it |
| `created` | string | `YYYY-MM-DD` |
| `priority` | string | `p0`…`p3`, with the default applied |
| `parent` | string | the epic it belongs to, or empty |
| `blocked_by` | array of string | the issues it waits on |
| `repro` | string | how to reproduce it; required on a bug |
| `acceptance` | string | what done looks like; required on a story |
| `question` | string | what is being answered; required on a spike |
| `reason` | string | why it was dropped |
| `resolution` | string | `wontfix`, `duplicate`, `works-as-intended` or `fixed-elsewhere` |
| `on_trunk` | bool | its folder exists at trunk |
| `reopened` | bool | trunk resolved it once and says open now |
| `contended` | bool | more than one branch claims it |
| `stale` | bool | a claim on it is older than `stale_days` |
| `claims` | array of `Claim` | the branches claiming it |
| `elsewhere` | array of string | the non-trunk refs whose copy differs from trunk's |
| `epic` | `Epic` or null | what its children add up to; null unless `type` is `epic` |
| `broken` | `Broken` or null | why trunk's copy will not decode; null when it does |

The five fields above `on_trunk` are on every issue rather than only on
`isu show`, because `isu ready --json | head -1` is meant to be the whole
briefing: an agent picking work up needs the acceptance criteria or the repro
before it needs anything else. The markdown body is one `isu show` away.

`status` is one of `done`, `dropped`, `awaiting triage`, `in progress`,
`reopened` or `open`. The rows are ordered and the first match wins, so a merged
issue whose claiming branch was never deleted reads `done`. `reopened` survives
as the annotation above whatever status wins, so the fact is never lost.

## Claim

One branch claiming one issue. A claim is a branch whose copy of the issue says
`resolved` where trunk says `open`; there is no other source.

| field | type | meaning |
|---|---|---|
| `ref` | string | the claiming branch, in full |
| `claimant` | string | who wrote the commit that flipped the state, or empty when it was not looked up |
| `email` | string | that author's address |
| `commit` | string | the commit that flipped the state — the first on the branch, never its tip |
| `when` | string | RFC 3339, that commit's author date, or empty |
| `age_seconds` | number | how long ago, and `0` when unknown |
| `stale` | bool | older than the repository's `stale_days` |

## Epic

| field | type | meaning |
|---|---|---|
| `children` | array of string | the ids naming this epic as their parent |
| `cycle` | bool | the rollup met this epic again while folding it |

## Broken

| field | type | meaning |
|---|---|---|
| `path` | string | the unreadable file, relative to the repository root |
| `error` | string | why it could not be read |

An issue is never dropped from the board for having one. A half-written issue
must not blind the whole board, least of all for the one issue somebody most
needs to hear about.

## Freshness

How old the newest remote-tracking ref is. Every derived status is a statement
about refs, so a board computed from a week-old fetch is a board about last
week — and two engineers with different fetch ages see different contention.

| field | type | meaning |
|---|---|---|
| `remote` | bool | there are remote-tracking refs at all |
| `newest` | string | RFC 3339 date of the newest, or empty |
| `age_seconds` | number | how old that is |
| `warn` | bool | past the repository's `fetch_warn_hours` |

## BoardPayload

`isu board`, one object.

| field | type | meaning |
|---|---|---|
| `trunk` | string | the ref isu read as trunk |
| `refs` | array of string | the other refs it read |
| `freshness` | `Freshness` | |
| `groups` | array of `Group` | statuses that matched something, in precedence order |

## Group

| field | type | meaning |
|---|---|---|
| `status` | string | the derived status |
| `issues` | array of `Issue` | most urgent first, then oldest, then by id |

## ShowPayload

`isu show <id>`, one object.

| field | type | meaning |
|---|---|---|
| `issue` | `Issue` | |
| `body` | string | the markdown below the frontmatter, verbatim |
| `attachments` | array of string | file names beside the README; contents are not read |
| `comments` | array of `Text` | |
| `children` | array of `Issue` | the issues naming this one as parent; empty unless it is an epic |
| `parent` | `Link` or null | the epic it belongs to |
| `blockers` | array of `Link` | what it waits on |
| `freshness` | `Freshness` | |

## Text

One comment file.

| field | type | meaning |
|---|---|---|
| `name` | string | `<date>-<author>-<nn>.md` |
| `body` | string | the file, verbatim |

## Link

Another issue this one names, with enough of it to render.

| field | type | meaning |
|---|---|---|
| `id` | string | |
| `title` | string | empty when the id resolves to nothing |
| `status` | string | empty when the id resolves to nothing |
| `known` | bool | the id names an issue this repository has |

An id that resolves to nothing is reported rather than dropped. What to do about
it is `isu check`'s call, in M5.

## Write

What every command that changes the repository prints: `new`, `claim`,
`unclaim`, `resolve`, `drop`, `comment`, `triage` and `init`.

One shape rather than eight. Somebody who has run `isu claim` and `isu resolve`
should not have to learn two answers to "what did you just do".

| field | type | meaning |
|---|---|---|
| `id` | string | the issue that changed; empty for `init`, which names none |
| `branch` | string | the branch written to, or empty when the change went into the working tree |
| `commit` | string | the commit written, or empty when the change was left uncommitted |
| `paths` | array of string | what changed, relative to the repository root |
| `pushed` | bool | the branch reached the remote |

Which commands fill which:

| command | `branch` | `commit` | `pushed` |
|---|---|---|---|
| `new` | `report/<id>` | yes | no |
| `new --no-branch` | — | — | no |
| `claim` | `isu/<id>` | yes | yes |
| `unclaim` | `isu/<id>` | yes | yes |
| `resolve` | the current branch | yes | no |
| `drop` | the current branch | yes | no |
| `comment` | — | — | no |
| `triage` | `triage/<id>` | yes | no |
| `triage --push` | trunk | yes | when there is a remote |
| `init` | — | — | no |

## CheckPayload

`isu check`: which rules ran, and what they found.

| field | type | meaning |
|---|---|---|
| `trunk` | string | the ref the run treated as trunk |
| `head` | string | the ref under review, or empty when there is none — a run on trunk, or a repository with no commits |
| `checks` | array of string | the names of the checks that ran, in name order |
| `findings` | array of `Finding` | what they found, failures first |
| `failures` | number | how many findings are failures |
| `warnings` | number | how many are warnings |
| `ok` | bool | nothing failed |
| `worktree` | bool | the rules read the issues on disk rather than at a ref |
| `freshness` | `Freshness` | how old the refs it read are |

Exit status is `1` when `ok` is false and `0` when it is true. **A warning does
not fail a run**: two people about to do the same work is worth saying and is
not a reason to refuse a pull request.

## Finding

One thing a check found.

| field | type | meaning |
|---|---|---|
| `check` | string | which rule found it |
| `severity` | string | `fail` or `warn` |
| `id` | string | the issue it is about, or empty when it is about the repository |
| `path` | string | the file it is about, relative to the repository root, or empty |
| `message` | string | what is wrong, in one sentence |

`check` is a stable name. A pipeline that greps for one is reading a contract,
so a rule is renamed the way a JSON field is: it is not.
