---
schema: 1
id: ISU-t5tjsg
title: M5-S4 · Owner immutability
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ams73p
blocked_by: ISU-xgyqb7
acceptance: accountability cannot be reassigned by anything that isn't a person.
---
**Done** #11, 2026-09-01. It reads the commit author rather than the diff, because the
asymmetry is the whole rule: the same one-line change is fine from a person and is not fine from
the thing they are supervising. An author is matched against `agents:` by name as written and by
address without regard to case — two display names are two decisions somebody made, where two
spellings of an address are one mailbox. A branch is not one author, so the finding lands on the
commit that did it and a person's commits beside it neither excuse it nor are blamed for it.
**Branch** `isu/M5-S4-owner-immutability`
**Build** if the commits on this branch change `owner:` and their author is in the configured
`agents:` list in `.isu.yml`, that is a `fail`. Humans may change it freely.
**Tests first** agent-authored owner change fails; human-authored passes; agent changing
`state` but not `owner` passes; a mixed-authorship branch fails on the agent's commit.
**Done when** accountability cannot be reassigned by anything that isn't a person.
