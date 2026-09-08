# Derived statuses

Status is never stored. It is computed, on every command, from what trunk and every branch say
about an issue — which means it cannot be stale, cannot be forgotten and cannot disagree with
the repository.

## The table

**The rows are ordered and the first match wins.** Several of them overlap, and without a
stated precedence a merged issue whose claiming branch was never deleted would match both
`done` and `in progress`.

| status | rule |
|---|---|
| `done` | trunk has `state: resolved` |
| `dropped` | trunk has `state: dropped` |
| `awaiting triage` | the folder exists on a branch and not on trunk |
| `in progress` | some branch has `state: resolved` where trunk has `open` |
| `reopened` | trunk has `state: open`, and some earlier trunk commit had `resolved` |
| `open` | on trunk with `state: open`, and no branch claims it |

Terminal trunk state beats every claim, so a finished issue reads `done` whether or not the
branch that claimed it was ever tidied up. `in progress` beats `reopened`, because somebody
actively re-fixing an issue needs to show as worked rather than as merely broken again — but
**`reopened` survives as an annotation** on whatever status wins, so the fact is never lost.

Here is all of it on one board:

```console
$ isu board
main · 9 issues · remote refs 3h ago

done (2)
  APP-x4h7vb  p2  spike  What does a failed payment cost us?  dana
  APP-2mdz8k  p3  chore  Upgrade the linter                   sam

dropped (1)
  APP-8ptr5s  p2  chore  Rewrite the CSS in another framework  sam

awaiting triage (1)
  APP-63kqwn  p0  bug  Sign-up page 500s on Firefox  sam

in progress (1)
  APP-9cx2rt  p2  story  Show the sign-up queue on the board  priya · claimed by Priya Raman

open (4)
  APP-7f3akq  p1  bug    Login retries drop the second attempt   dana
  APP-40b1cc  p2  epic   Make sign-up reliable                   dana · 5 children
  APP-b5n3kt  p2  story  Send a receipt after the first invoice  priya
  APP-5wq7dn  p3  chore  Publish a status page                   sam
```

## Awaiting triage

The folder exists on a branch and has never reached trunk. This is the status a tracker with a
database cannot express at all: the issue has been *reported*, in a pull request somebody has
to review, and it is not yet part of the repository's account of itself.

It is also why `isu new` writes onto `report/<id>` rather than into your working tree. A report
is a proposal like any other, and it goes through the same review as a code change.

## In progress, and what a claim is

`in progress` has exactly one source, and it is the claim itself: `isu claim` writes
`state: resolved` on `isu/<id>` before any work starts. A claimed issue and an issue somebody
resolved on a branch without claiming are therefore the same observable fact, read the same
way. There is no advisory side-channel to reconcile against what the branches say, because
there is no second signal.

A branch that exists *without* that flip is deliberately not `in progress`. It is somebody's
work on a branch, and until they say so by claiming, the board does not speak for them. That is
also what makes `isu unclaim` mean anything: it flips the state back and leaves the branch
standing.

Four annotations ride along on `in progress`:

- **claimant** — the author of the commit that flipped the state, which is the *first* commit
  on the claiming branch and never its tip. The tip moves every time the claimant pushes more
  work, so a claim read from it would never age and `stale_days` would never fire.
- **age** — that commit's author date.
- **stale** — older than `stale_days`, which defaults to 7.
- **contended** — two branches claiming the same issue. That is a warning and not a failure:
  two people about to do the same work is worth saying and is not a reason to refuse a pull
  request.

```console
$ isu show APP-9cx2rt
APP-9cx2rt  Show the sign-up queue on the board

  status      in progress
  type        story
  state       open
  owner       priya
  created     2026-02-21
  priority    p2
  acceptance  the board shows queue depth and the oldest waiting account
  parent      APP-40b1cc  Make sign-up reliable (open)
  branches    refs/heads/isu/APP-9cx2rt
  trunk       main

  claimed by Priya Raman on refs/heads/isu/APP-9cx2rt, 2026-04-14T04:15:00Z

remote refs 3h ago
```

## Reopened

Trunk says `open` now, and some earlier trunk commit said `resolved`. That is read from file
content at trunk commits rather than from commit messages, because a squash merge collapses
authorship and does not touch the file.

A deletion in between does not break the chain. Issues are files, so somebody can `git rm` a
folder and commit it — there is no command for that, but nothing prevents it either — and an
id can come back afterwards, whether by reverting that commit or by re-running an import. An id
is permanent from creation, so the same id is the same issue, and trunk did resolve it once.

## Epics

An issue of `type: epic` takes its status from a fold over its children: all terminal means
resolved, unless all are dropped, which means dropped; anything else is open. An epic with no
children is a [check failure](checks.html) rather than a status, because a fold over nothing has
no honest answer.

```console
$ isu show APP-40b1cc
APP-40b1cc  Make sign-up reliable

  status    open
  type      epic
  owner     dana
  created   2026-02-21
  priority  p2
  trunk     main

children
  APP-63kqwn  awaiting triage  Sign-up page 500s on Firefox
  APP-7f3akq  open             Login retries drop the second attempt
  APP-9cx2rt  in progress      Show the sign-up queue on the board
  APP-x4h7vb  done             What does a failed payment cost us?
  APP-b5n3kt  open             Send a receipt after the first invoice

Everything between the landing page and the first invoice.

remote refs 3h ago
```

## Freshness

Every derived status is a statement about refs, so a board computed from a week-old fetch is a
board about last week — and two engineers with different fetch ages see different contention.
Every board and every `isu show` therefore ends with how old the newest remote-tracking ref is,
and says `run with --fetch` once that passes `fetch_warn_hours`.
