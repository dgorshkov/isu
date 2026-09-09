---
schema: 1
id: ISU-8t3xyb
title: M1-S4 · ID generation and `.isu.yml`
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-7076wc
blocked_by: ISU-13maqz
acceptance: generating an id needs neither the network nor a read of `issues/`. Collision regeneration is deliberately **not** here — it needs the loaded repo, so it belongs to `isu new` in M4-S3.
---
**Done** #5, 2026-08-26. **The `NewID` signature below was corrected as this story was
built** — it read `NewID(title, owner, created)`, which cannot return an id because it is
never told the prefix. The separator in the id hash was pinned to a NUL byte in the data model for
the same reason: it was written as `‖` and never defined. "Zero reads" is proven in two halves
— the id is identical inside a 5,000-issue repository and in an empty directory, and identical
again with the working directory deleted out from under the process. Config is read and
validated; **nothing writes `.isu.yml`** — see the open question under Configuration.
**Branch** `isu/M1-S4-ids`
**Build** config loading and validation against the `.isu.yml` table in the data model, plus
`NewID(prefix, title, owner, created)` implementing the token scheme, and
`config.Config.NewID(title, owner, created)` over it for callers that already hold the
configuration. **An id is `<PREFIX>-<token>`, so the generator has to be told the prefix**;
earlier drafts of this line omitted it and described a function that cannot return an id.
**`NewID` is pure** — it reads nothing, knows about no other issue, and cannot detect a
collision, because detecting one means loading the repository and the git layer does not exist
until M2. No counter, no scan for a maximum, no `isu renumber` — ids are permanent from
creation.
**Tests first** the same inputs plus different random bytes give different ids; the alphabet
never emits `i`, `l`, `o` or `u`; `NewID` issues zero reads against a 5,000-issue fixture; an
imported `PROJ-1234` validates as an id even though nothing would ever generate it; every key
in the config table round-trips with its default applied, an unknown key is refused, and a bad
value is refused with the key named.
**Done when** generating an id needs neither the network nor a read of `issues/`. Collision
regeneration is deliberately **not** here — it needs the loaded repo, so it belongs to `isu new`
in M4-S3.
