---
schema: 1
id: ISU-stb9h3
title: M3 · Derivation
type: epic
owner: dmitry
created: 2026-08-23
priority: p2
---
**Status** done — all five stories landed in one pull request rather than five. That was asked
for explicitly, as it was for M1 and M2, and the working agreement now allows it outright. M3 is where the size of
a group starts to cost something: this milestone is the product, and it drew more review time
than anything before it. The milestone boundary rule applies as ever — M4 does not
start without explicit approval.

**Two corrections and one question came out of the tests**, and all three are recorded where
the decision was made rather than only here: the claim lookup in the data model and M3-S3 returned
the branch tip and is corrected above; the merge-then-revert case reads a different status
depending on whether the branch was deleted, which M3-S4's test list did not expect; and an
issue deleted at trunk and reported again is a reopen, which M3-S4 asked about and the data model
now answers. Nothing in M3 is left open.

**Claim refs are not loaded, and now never will be.** M2's note deferred them to M3-S3, "which
is where what they mean is decided" — and what they mean is nothing: the Claims section was
rewritten in #7 to claim by branching and flipping the state, so there is no `refs/claims/`
namespace to read. `gitx.ForEachRef` can still list one, which is a general wrapper doing its
job rather than a loose end.
