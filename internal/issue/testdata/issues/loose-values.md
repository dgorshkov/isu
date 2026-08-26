---
schema: 1
id: ISU-6h2ldq
title: Spelled the way a human typed it, not the way isu would
type: bug
state: open
owner: dmitry
created: 2026-08-24
priority:
parent:
blocked_by: ISU-39ka2p,ISU-2kd8vw ,  ISU-40b1cc
repro: POST /session five times and watch the fourth 500
---
Every value above is legal and none of it is spelled the way this package
would choose to write it. An optional key present with an empty value is not
the same as an absent one, and a comma list without spaces is not an invitation
to reflow it.
