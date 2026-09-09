# Importing from GitHub Issues

> **In this build.** Milestone 7 implemented the mapping this page describes, and every
> `isu import` below is run against a recorded issue list while this page is built. Nothing
> here has been near github.com: a page that needs a network to build is a page that stops
> building.

GitHub Issues is the one importer version 1.0.0 will ship, and that is a decision rather than
an omission. Whoever is adopting isu is already in a git repository, that repository is almost
always on GitHub, and GitHub answers for free — with no licence and no administrator — two
questions Jira answers only through an integration somebody had to install: **which pull
request closed this issue**, and **what is this issue blocked by**. It can also be read without
credentials, which is what lets the importer be tested against real public repositories rather
than against a fixture nobody can check.

Jira and Linear are on the [out-of-scope list](not-doing.html) as deferrals. The importer's
source interface is the seam they arrive through.

## Ids

A GitHub key is `#1234`. It is repo-scoped, it shares its sequence with pull requests, and it
is not a folder name — `#` and `/` are not in the id character set. So the key becomes
`<PREFIX>-1234`, taking the prefix from `.isu.yml` and keeping the number, which is the half a
human recognises and the half every link contains. `owner/repo#1234` and the issue's URL are
recorded in `source.yml` beside the issue, so nothing about the source is lost.

A key that is *already* a legal id is kept verbatim. `PROJ-1234` stays `PROJ-1234`, because the
thing your team has been writing in commit messages for years should still be legible in the
id.

Milestones are numbered from their own sequence, so they take `<PREFIX>-M<number>`: `APP-M3` and
`APP-3` are different issues and must not collide.

## The mapping

| GitHub | isu | note |
|---|---|---|
| issue type, or a configured label map | `type` | Bug → `bug`, Feature → `story`, Task → `chore`; `chore` when neither answers |
| milestone | an epic, and the issue's `parent` | an issue belongs to at most one, so there is no collapse to design |
| `open` | `state: open` | |
| `closed`, `state_reason: completed` **or absent** | `state: resolved` | the field only exists since 2022, and everything older is an ordinary closed issue |
| `closed`, `state_reason: not_planned` | `state: dropped`, `resolution: wontfix` | |
| `closed`, `state_reason: duplicate` | `state: dropped`, `resolution: duplicate` | |
| `reopened` | `state: open` | isu derives reopening from trunk history and will not read it from a field it would then have to keep in sync |
| issue dependencies | `blocked_by` | a blocker outside the import goes to `source.yml` rather than becoming an id that resolves to nothing |
| first assignee, else `--owner` | `owner` | the author is recorded in `source.yml` and is deliberately not the owner |
| comments | `comments/<date>-<author>-<nn>.md` | the sequence is why three comments from one person on one day stay three files |
| everything else | `source.yml` | issue fields, sub-issue trees, attachment links, the author, the milestone's own state |

**Pull requests are not issues.** The REST list returns both, and they are told apart by the
`pull_request` key; dropping them is the first thing the importer does and the classic bug in
every one that skips it.

**A milestone with no imported children is not written at all**, because an empty epic is a
check failure — and `--state open` empties every milestone whose issues are all closed.

**Sub-issues do not become `parent`.** isu has one `parent`, it must name an epic, and the
milestone is what it is spent on. A GitHub tree can be eight levels deep where an epic is one,
so the hierarchy is written whole to `source.yml` and its depth and size are reported by the dry
run. That is the largest thing version 1.0.0 knowingly declines to model, and recording it
losslessly is what makes modelling it later a story rather than a re-import.

## The fields the schema requires and GitHub does not have

`repro`, `acceptance` and `question` are required by type, GitHub supplies none of them, and a
naive import would write thousands of files that fail validation on the first `isu check`. Each
is written as a provenance line — `repro: imported from owner/repo#1234; see the body` — and
the dry run counts them, so nobody mistakes archaeology for content.

The rejected alternative was to call everything a `chore`, which validates trivially and throws
away the bug/story distinction across the entire history in one move.

## Attachments are recorded, not downloaded

A GitHub attachment is a `github.com/user-attachments/assets/…` link inside the markdown. On a
private repository it cannot be fetched with a personal access token or a GitHub App token at
all: the asset wants a browser session. An importer whose completeness depends on winning that
race is one that half-works on exactly the repositories people most want migrated — so the
links stay in the body byte for byte, they are listed in `source.yml`, and the dry run says how
many there are.

## What running it looks like

A dry run is the default and writing requires `--write`. The dry run reports counts, coverage
and samples without touching the repository: how many issues, how many types it could not
place, how many owners it could not find, how many provenance lines it had to write, how many
attachment links it recorded, and what the API budget cost.

The runs below read `acme.json`, a recorded issue list. `--fetch-only` is what writes one, and
it is the only step that wants a network; after that the same command gives the same answer on
a train, and a reviewer can see what the numbers were computed from.

```console
$ isu import github --dump acme.json --owner dana --samples 1
import github → acme/acme-app, as APP-*
  would write 6 folders from 6 issues, 1 of them epics
  types: bug 2, chore 2, epic 1, story 1
  states: (fold over its children) 1, dropped 2, open 2, resolved 1
  2 comment files
  1 attachment link, recorded and not fetched
  24 values with nowhere in the schema, kept in source.yml
  3 required fields written as a provenance line, not as content
  1 link pointing outside the import, recorded and not written
  resolving commits: closing pull request 1
  found: comments, dependencies, issue fields, issue types, milestones
  attachment links are recorded, not fetched: resolving one still needs github.com, because on a private repository the asset wants a browser session

skipped
  #7  a pull request, which the REST list returns beside the issues

APP-1 (#1)
  README.md  source.yml  comments/2026-01-13-priya-01.md  comments/2026-01-13-priya-02.md
  ---
  schema: 1
  id: APP-1
  title: Login retries drop the second attempt
  type: bug
  state: resolved
  owner: dana
  created: 2026-01-12
  parent: APP-M1
  repro: imported from acme/acme-app#1; see the body
  ---
  The retry reuses the first request's nonce, so the second POST is rejected before it reaches the handler.
  
  ![trace](https://github.com/user-attachments/assets/8b17c0d4)


nothing was written: pass --write when this says what you expected
```

`--owner` names who gets an issue GitHub left unassigned. `--samples` is how many issues are
shown in full, and it exists so that `would write 6 folders` is not the only thing anybody sees
before several thousand files arrive. `APP-M1` is the milestone: an epic, and what `parent` is
spent on.

`--state` asks for a subset, and what it declines stays on the record beside what it took:

```console
$ isu import github --dump acme.json --owner dana --state open --samples 1
import github → acme/acme-app, as APP-*
  would write 3 folders from 3 issues, 1 of them epics
  types: chore 1, epic 1, story 1
  states: (fold over its children) 1, open 2
  9 values with nowhere in the schema, kept in source.yml
  1 required field written as a provenance line, not as content
  2 links pointing outside the import, recorded and not written
  found: dependencies, issue fields, issue types, milestones
  attachment links are recorded, not fetched: resolving one still needs github.com, because on a private repository the asset wants a browser session

skipped
  #1  --state open does not ask for it
  #4  --state open does not ask for it
  #5  --state open does not ask for it
  #7  a pull request, which the REST list returns beside the issues

APP-2 (#2)
  README.md  source.yml
  ---
  schema: 1
  id: APP-2
  title: Show the sign-up queue on the board
  type: story
  state: open
  owner: priya
  created: 2026-01-19
  parent: APP-M1
  acceptance: imported from acme/acme-app#2; see the body
  ---
  Support cannot tell how many accounts are waiting.


nothing was written: pass --write when this says what you expected
```

The second run records two links pointing outside the import where the first recorded one. `#1`
is a blocker `--state open` did not bring, so it goes to `source.yml` rather than becoming an id
that resolves to nothing — which is the same rule that put `#41` there in both runs.

Two things are also recovered from history for issues that predate isu: which commit resolved
each one, at three tiers of evidence — a key in a commit message, a key in a merge commit's
branch name, a key in a squash subject — and, better than all three,
`closedByPullRequestsReferences`, which GitHub already stores and which arrives with the issue.
That is the `resolving commits: closing pull request 1` line above: `#1` was closed by pull
request `#7`, and no history had to be searched to know it.

Every tier match is checked against the set of numbers actually being imported and discarded
when it is not one of them, because `#456` in a squash subject almost always names a pull
request and `#1234` turns up in prose about nothing at all.

## Once, and one way

Import is one-way and one-time by design. There is no bidirectional sync with anything, and
there will not be: two systems that both believe they own the state are two systems that
disagree at three in the morning.
