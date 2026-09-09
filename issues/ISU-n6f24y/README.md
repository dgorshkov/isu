---
schema: 1
id: ISU-n6f24y
title: M5-S7 · Dogfooding switch
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ams73p
blocked_by: ISU-q4dy32
acceptance: isu tracks its own construction and its own CI enforces it. From here on every
---
**Done** #11, 2026-09-01. Twenty-nine issues: five epics, one per remaining milestone, and
twenty-four stories, one per remaining story heading — all through `isu new --no-branch`, with
`--parent` naming the milestone and `--blocked-by` naming the story before it, so that the first
story of a milestone waits on the milestone before it and an epic blocker is terminal when its
rollup is. Each story's `acceptance` is its own **Done when** sentence, which is what that line
has always been. **This pull request's own seven stories are flipped to `resolved` by `isu
resolve`**, one commit and one `Isu-Resolves:` trailer each, which is the mechanism this story
turns on for everything after it.

**The test reads the working tree where this document says trunk, deliberately.** An assertion
about trunk is one that cannot fail on the pull request that breaks it — trunk has not merged it
yet — so it would be green for the whole of the review and red immediately afterwards, which is
the one moment nobody is looking. Beside the four assertions this story asks for, it holds
the plan and `issues/` to each other in both directions: every story heading from M5 on has an
issue with that title, and every issue is one of those headings. Two documents saying the same
thing drift the moment nobody is checking.

`make dogfood` is the gate, and CI runs it against the base branch it fetches by name. The
binary is the one just built rather than a release: a pipeline that checked this repository with
last month's isu would pass the pull request that broke the checks.
**Branch** `isu/M5-S7-dogfood`
**Why** Last in this milestone on purpose. It needs `isu new` from M4-S3 to create the issues
and the whole check suite to keep them honest; converting any earlier means hand-maintaining
issue files with nothing verifying them.
**Build** convert every remaining story in this plan into an issue folder using
**`isu new --no-branch`**, so the whole conversion lands on this one branch and reaches trunk in
this one pull request — the default `report/<ID>` branch per issue would scatter fifty issues
across fifty branches that the test below, which reads trunk, could never see. Milestones become
`--type epic`; every story is created with `--parent` naming its milestone, and `blocked_by`
encodes the order. Turn `isu check` on for this repository's own pipeline.
**Tests first** a test loading `issues/` from trunk asserting every issue validates, the
dependency graph is acyclic, every epic has at least one child, and every non-epic names a
`parent` that is an epic. That last one is an assertion about **this repository's tree**, which
the conversion controls — `parent` stays optional in the schema, because a repository with no
epics at all is a perfectly good repository.
**Done when** isu tracks its own construction and its own CI enforces it. From here on every
pull request also flips the issue file of each story it carries to `resolved` — and M5-S3 turns
that into a failing check if the pull request contains nothing else.

---
