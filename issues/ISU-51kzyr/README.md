---
schema: 1
id: ISU-51kzyr
title: M2 · Git layer
type: epic
owner: dmitry
created: 2026-08-23
priority: p2
---
**Status** done — all five stories landed in one pull request rather than five. That was
asked for explicitly, as it was for M1, and the working agreement now allows it outright. What the working agreement still asks for
is the judgement, milestone by milestone: group what one reviewer reads in one sitting, not
whatever happens to be adjacent. The milestone boundary rule still applies: M3 does not start
without explicit approval.

**Claim refs are not loaded yet.** M3-S1 names them among its inputs, but M0-S4's note
already defers them to M3-S3, "which is where what they mean is decided", and M2 did not
skip ahead of that. `gitx.ForEachRef` lists `refs/claims/` today and has a test that does;
`repo.Board` grows a field for them in M3-S3, which is the loader growing rather than
derivation reaching for git — §M3-S1's rule, kept.

**M3-S3 decided, and the answer was that there is nothing to load.** The Claims section was
rewritten in #7 to claim by branching and flipping the state, so no `refs/claims/` namespace
exists to read and the field this paragraph promised was never added. What M3-S3 did add to
`repo.Board` is `Changed`, for a different reason entirely — see M3-S1.
