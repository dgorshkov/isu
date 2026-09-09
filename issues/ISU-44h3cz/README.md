---
schema: 1
id: ISU-44h3cz
title: M4-S4 · `isu claim` and `isu unclaim`
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-6mkqq5
blocked_by: ISU-t97sg3
acceptance: the deterministic rejection test passes and `make stress` exists for the rest.
---
**Done** #9, 2026-08-31. **This story found the hole in the claim design, and the data model is corrected
above**: the stress run put a hundred clones on one issue and thirty of them won, because two
claimants under one identity in the same second write the same commit. The claim commit now
carries an `Isu-Claim:` nonce, the race has one winner, and a deterministic regression test claims
the same issue twice under one identity and asserts the commits differ. Everything else is as
specified: three steps in order, **no checkout at any point** — the commit is built through a
temporary index, so claiming works mid-edit and a lost race leaves nothing behind — the branch
deleted when the push loses, and `unclaim` flipping the state back without deleting anything. The
hundred-run variant lives behind `//go:build stress` and `make stress`, out of the default suite.
**Branch** `isu/M4-S4-claim`
**Build** the three-step claim from the data model, in that order — branch, the state flip committed
with subject `claim <ID>`, push. Claim failure must be fast, quiet and exit non-zero, naming
the branch's holder. There is **no ref-namespace failure left to distinguish and no
`--no-claim` mode to build**: the claim is a branch, so a rejected push is a lost race and
nothing else. `unclaim` flips the state back to `open` and pushes, and never deletes the
branch.
**Tests first** the rejection path **deterministically**: claim from one clone, then claim the
same issue from a second and assert the failure names the holder and exits non-zero. **Assert
the pushed branch is not merely the trunk tip** — a push of the bare tip is a no-op
fast-forward git accepts from both claimants, and the state flip is the whole reason that
cannot happen here, so the test that would have caught the earlier design belongs in this one.
Repeating a network operation a hundred times per CI run buys confidence in the network, not
the code — so the hundred-run stress variant lives behind `//go:build stress` and is out of the
default suite. Also: claiming an already-terminal issue is refused; unclaim leaves the branch
and any work on it in place, and leaves the file byte-identical to what trunk says; **claiming
with the remote detached fails without leaving a local branch behind**.
**Done when** the deterministic rejection test passes and `make stress` exists for the rest.
