---
schema: 1
id: ISU-t97sg3
title: M4-S3 · `isu new` and `isu ready`
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-6mkqq5
blocked_by: ISU-sbhnj0
acceptance: `isu ready --json | head -1` gives an agent everything it needs to start.
---
**Done** #9, 2026-08-31. Three decisions, all forced by "new produces a valid issue" meeting the
schema in the data model. **`new` requires the field its type requires**: a bug with no `repro` is not
a valid issue, so refusing it is the schema surfaced at the command rather than at `isu check`,
and the alternative was writing a placeholder into a required field, which is inventing content
nobody wrote. **`--owner` defaults to git's `user.name`** and asks when git has none, because
`owner` is required and there is exactly one honest guess about who is filing. **`ready` excludes
epics**: an epic has no state of its own and is finished when its children are, so it is never a
thing to pick up — an agent handed one would have nothing to do and no way to say it was done. It
excludes an unreadable issue for the same shape of reason.
**Branch** `isu/M4-S3-new-ready`
**Build** `new` scaffolds a folder, generates an id per M1-S4 and **regenerates it if the token
collides with any id it can see** (trunk plus local refs — this is the story that has the loader
M1-S4 lacks), requires `--title`, stamps `created`, and defaults `priority` to `p2`. It also
takes `--type` (default `bug`) and `--parent`; **`--type epic` omits `state:`**, and is the only
way to create an epic. By default it creates a `report/<ID>` branch and stages it for a pull
request; **`--no-branch`** writes into the working tree instead, which is what a bulk conversion
needs and what M5-S7 uses.
`ready` lists open issues whose `blocked_by` are all terminal — **an epic blocker is terminal
when its rollup is**, since an epic has no `state:` of its own — ordered by priority then age,
`--json` by default for agents.
**Tests first** `new` produces a valid issue and a branch not on trunk (status: awaiting
triage); `new` without a title is refused; two `new` runs in the same second produce different
ids; a seeded collision with an existing id is regenerated rather than returned; `--type epic`
produces an issue with no `state:` that passes M1-S2, and any other type without one fails;
`--no-branch` leaves the repository on the branch it started on; `--parent` naming a non-epic is
refused; `ready` excludes blocked, dropped, claimed and contended issues; `ready` treats an
issue blocked by a fully-resolved epic as ready and one blocked by a half-done epic as not;
`ready` puts a `p0` ahead of an older `p2`.
**Done when** `isu ready --json | head -1` gives an agent everything it needs to start.
