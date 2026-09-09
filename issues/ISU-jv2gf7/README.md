---
schema: 1
id: ISU-jv2gf7
title: M3-S3 · Claimant, age, staleness, contention
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-stb9h3
blocked_by: ISU-w3xnyq
acceptance: contention and staleness come out of what M2 already loaded plus one log per claiming branch, and nothing in `internal/model` runs git.
---
**Done** #8, 2026-08-27. **The lookup below was corrected as this story was built**, and the
correction is written up in the data model: `--reverse --max-count=1` returns the branch tip, which
that section calls the wrong answer in the paragraph above the one that spelled it. The range
is walked and its first record taken instead. The split between the two packages is the other
thing this story settled: *which* refs claim is a pure question about what was already loaded,
so `model.ClaimRefs` answers it, and *who* claimed costs a git process each, so
`repo.LoadFirstCommits` does that over the refs it named. Asking the loader to work both out
would have made it walk every branch in the repository. A ref with no first commit — nothing
ahead of trunk — is a lookup with no answer rather than a failure, and a claim whose first
commit was not looked up is still a claim: the file is the claim, and the lookup only names who
made it, so dropping the row would hide a claim to protect an annotation. **A zero
`Input.Config` means the default rather than zero days**, added after review: `StaleAfter()` of
zero makes every claim stale the instant it is made, and `Derive` already defaulted the sibling
zero value `Now`. A zero value that silently flags every row on the board as abandoned is a
footgun, and documenting it is not as good as not having it.
**Branch** `isu/M3-S3-claims`
**Build** a claim is a branch whose issue file says `state: resolved` where trunk says `open`.
The claimant is the author of the **first commit on that branch** and the claim time is its
author date; staleness is that date against `stale_days`. An issue is contended when more than
one branch claims it.
**The lookup belongs to the loader, not to this package.** `internal/repo` grows a call that
runs `git log <trunk>..<branch> --reverse` for each claiming branch and takes the first record,
handing the result over — one process per claiming branch, linear in refs like everything else
the board does. M3-S1's rule that derivation never calls git has no exceptions, and this is the
first story that would have been tempted to make one.
**Tests first** a single claim names the claimant and the date the state was flipped, **not the
branch tip's date** — assert with more work pushed on top, which moves the tip and must not
move the claim; two claiming branches by different authors read as contended with both named; a
claim backdated 11 days reads as stale; a branch that exists without the state flip is not a
claim; unclaim — the state flipped back to `open`, branch still standing — stops reading as
in progress; a claiming branch deleted after merge leaves the issue reading `done` from trunk.
**Done when** contention and staleness come out of what M2 already loaded plus one log per
claiming branch, and nothing in `internal/model` runs git.
