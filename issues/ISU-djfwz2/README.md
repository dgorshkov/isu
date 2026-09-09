---
schema: 1
id: ISU-djfwz2
title: M7-S4 · GitHub: issues, types, milestones and state
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-2wyps1
blocked_by: ISU-4jsxyw
acceptance: the imported tree passes `isu check` with zero failures and makes no request per issue.
---
**Done** #15, 2026-09-03. **Three decisions this document left open, each recorded here because
the reason matters more than the answer.**

**The dump is the REST list, and `gh`'s spelling is read as an alias.** This story says "a saved
`gh issue list --json` dump". `gh` writes camelCase for four keys and the REST API writes
snake_case, and `--fetch-only` records what the API returned — so the REST shape is the format
and `stateReason`, `createdAt`, `url`, `author` and `issueType` are accepted beside it. `comments`
is an integer in the REST list and an array in a dump that carries them, so it is held raw and
decoded only when it is the second; a count is not a comment, and refusing the import over one
would refuse every REST dump.

**The sub-issue hierarchy is free and the dependencies are not, and the difference is which of
them the list carries.** This was written the other way round first and the documentation
corrected it: every row of the REST list carries `parent_issue_url`, so the whole tree is in the
pages already read and costs nothing — no request per issue and no dump extension needed to have
it. `sub_issues` stays readable as the other direction of the same link, for a dump somebody
assembled from the sub-issues endpoint, and both go through one `link` that ignores anything that
would not be a tree: an issue outside the import, an issue under itself, a second parent for a
child that has one. Dependencies have no such field — GitHub answers them at
`/issues/{n}/dependencies/blocked_by`, per issue — so they are read where a dump carries them and
never fetched, because 5,000 issues would be 5,000 requests against a budget of 5,000 an hour.

**A milestone's first copy wins.** The REST list embeds the whole milestone on every issue in it,
so the copies are one object repeated; a dump somebody assembled by hand may have thinned the
later ones, and overwriting would lose the description the first one carried.

**A 403 is not a rate limit on its own, and reading it as one was a defect.** This story says rate
limits are handled rather than hoped for, and 403 is what a secondary rate limit returns — so the
first version waited one out on every 403 it saw. GitHub answers 403 for "you may not read this"
as well, and the two want opposite handling: one is worth waiting out and the other will never
improve. Pointed at a repository the token could not read, that cost five requests and eleven
seconds of backoff before a message that was already correct on the first one — against the
budget the waiting exists to protect. A 403 is now throttling only when the response says so: a
`retry-after`, a spent `x-ratelimit-remaining`, or a body that mentions the limit it is about.
Eleven seconds became three tenths of one, measured on the same call.

**No part of this milestone has been run against github.com.** M7-S5 asks for a recorded
transcript and never the live API, and that is what the suite reads; the API path is exercised
against a server the tests start, which is real HTTP over a real socket and is not real GitHub.
The two shapes this milestone had wrong were found by reading the documentation rather than by
running anything, and the 403 above was found by one call that never got past the proxy. **A
first-contact story in the shape of M4-S8 is what would close this**, and it is not in the plan.

Everything else landed as specified. Pull requests are dropped first and `null` under the
`pull_request` key is not a pull request — which matters because a dump this importer wrote
round-trips the key as exactly that. The type falls through the organisation's own type, then the
label map, then `chore`, and an issue GitHub's own type placed never puts its labels on the
"could not place" list. An issue's `blocked_by` naming another repository is keyed
`owner/repo#5`, so it stays outside the import rather than resolving to this repository's `#5`.
**Branch** `isu/M7-S4-github-core`
**Build** read issues from the GitHub API, or from a saved `gh issue list --json` dump —
`--fetch-only` writes that dump, the tests read one, and an import is therefore reproducible
without a network and reviewable as a diff. **Pull requests are not issues.** The REST list
returns both and they are told apart by the `pull_request` key; dropping them is the first
thing this importer does and the classic bug in every one that skips it.

- **Type.** The organisation's issue type where the repository has one, a configured label map
  where it does not, `chore` where neither answers. GitHub's own defaults map Bug to `bug`,
  Feature to `story` and Task to `chore`; every other type and every label is the map's
  business, and the dry run lists what it could not place.
- **Epics are milestones.** A milestone is a named container whose progress is a fold over the
  issues in it, which is exactly what an isu epic is, and an issue belongs to at most one — so
  it lands in the single `parent` field with no collapse to design. The epic is written
  **without `state:`** like every epic, the milestone's description becomes its body, and its
  own open/closed state and due date go to `source.yml`, because here the fold is what decides.
  Its id is `<PREFIX>-M<number>`: milestones are numbered from their own sequence, so `ISU-M3`
  and `ISU-3` are different issues and must not collide. **A milestone with no imported children
  is not written at all** — M5-S2 makes an empty epic a check failure, and `--state open` empties
  every milestone whose issues are all closed.
- **Sub-issues do not become `parent`.** The field is spent on the milestone, isu has one, and a
  sub-issue tree is eight levels deep where an epic is one. The hierarchy is written whole to
  `source.yml` and its depth and size are reported by the dry run. This is the largest thing
  v1.0.0 knowingly declines to model, it is on the out-of-scope list under its own heading, and
  recording it losslessly is what makes modelling it later a story rather than a re-import.
- **State.** `open` becomes `open`. `closed` with `state_reason: completed`, **or with no
  `state_reason` at all**, becomes `resolved` — the field only exists since 2022 and everything
  older is an ordinary closed issue rather than an unknown. `not_planned` becomes `dropped` with
  `resolution: wontfix`, `duplicate` becomes `dropped` with `resolution: duplicate`, and
  `reopened` becomes `open`, because isu derives reopening from trunk history and will not read
  it from a field it would then have to keep in sync.
- **`blocked_by` is `blocked_by`.** GitHub's dependencies say what isu's field says. A blocker
  outside the import goes to `source.yml` rather than being written as an id that resolves to
  nothing, which M5-S2 would fail on.
- **Owner.** The first assignee; failing that `--owner`; failing that the import refuses and the
  dry run says how many issues have neither. The author is recorded in `source.yml` and is
  deliberately not the owner: the person who filed a bug is usually not the person answerable
  for it, and M5-S4 makes `owner` expensive to correct afterwards.
- **The type's required field.** `repro`, `acceptance` and `question` are required by the
  schema, GitHub supplies none of them, and a naive import would emit thousands of files that
  fail `Validate()` on the first `isu check`. Each is written as a provenance line —
  `repro: imported from owner/repo#1234; see the body` — and the dry run counts them so nobody
  mistakes archaeology for content. The rejected alternative was to call everything a `chore`,
  which validates trivially and throws away the bug/story distinction across the entire history
  in one move.
- **Rate limits are handled, not hoped for.** Five thousand points an hour, a hundred concurrent
  requests, and a cap per minute besides. A 5,000-issue import fits comfortably inside that and
  only if it batches; on a 403 or a 429 the importer waits out `retry-after`, or
  `x-ratelimit-reset` when there is no `retry-after`, then backs off, and the dry run reports
  what the budget cost.

**Tests first** a fixture dump covering issue types and a repository with none, milestones
including an empty one, a four-level sub-issue tree, every `state_reason` including its absence,
dependencies pointing inside and outside the import, and unassigned issues. Assert pull requests
in the list are not imported; assert imported milestones carry `type: epic` and no `state:`;
assert every dropped issue has a `resolution` from the enum and a `reason`; assert an issue whose
milestone was skipped emits no `parent`; assert an out-of-import blocker leaves `blocked_by`
absent rather than dangling; **assert the whole import makes no per-issue request for anything
the list already carried** — a process count in the spirit of M2-S5, against a transport that
counts.
**Done when** the imported tree passes `isu check` with zero failures, and 5,000 issues are read
in pages of a hundred with no request per issue.
