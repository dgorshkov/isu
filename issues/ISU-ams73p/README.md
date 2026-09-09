---
schema: 1
id: ISU-ams73p
title: M5 · Checks
type: epic
owner: dmitry
created: 2026-09-01
priority: p2
---
**Status** done — all seven stories landed in one pull request rather than the two or three
this document recommended at the head of M4. That was asked for explicitly, and the working agreement allows it;
it is worth recording that the recommendation was right about the shape and wrong about the
seam. The seven checks really are independent, but they are all one Input away from each
other, and the two loaders that build that Input — what a branch proposes over trunk, and what
lives beside each issue — are M5-S1's whether they are used by one check or by four.

**The command this milestone needed did not exist yet.** `isu check` is not in M4's list of
eight, because until there was a check engine there was nothing for it to run; it arrives here,
with a row in the JSON contract, a golden help file that lists the rules themselves, and the
same `--json` obligation as everything else.

**Two flags are decisions this document did not make, and both are the pre-commit hook's
fault.** `--scope tree|branch|all` exists because the hook cannot ask the branch questions: at
pre-commit time the change being committed is not a commit yet, so those rules would be
answered from the commits already on the branch — and on a claiming branch that answer is "you
resolved an issue and wrote no code", which would block the very commit that writes the code.
`--worktree` exists because the tree questions have the same problem in a worse form, and
M5-S6 records what testing it found: a hook reading refs refuses the commit that fixes what it
is complaining about, which is a deadlock and not a gate.

**One defect and one carve-out came out of the rules themselves.** The duplicate-id check
reported one issue claimed on two branches as two issues wearing one id, found by M5-S5's first
two-branch fixture; and the evidence check does not demand code from an issue a branch
*created* in a terminal state, because that is what an import is and M7 would be
unimplementable otherwise. Both are recorded at the story that owns them.

**M4-S8's limitation is now load-bearing and still open.** The board reads `refs/heads/`, so a
claim that reached this clone as `refs/remotes/origin/isu/<ID>` is invisible — and M4-S8 asked
for a story of its own "before M5-S5 reports contention to anybody". That story has not been
written and this milestone did not invent it: instead every contention and staleness warning
says which refs it read, so a local run that disagrees with CI is self-explaining rather than
confidently wrong. **The story is still wanted, and the proposal is in the pull request.**

**The branch is not the one the working agreement names.** A branch carrying several stories is named for the
range and its subject — `isu/M5-S1-S7-checks` — and this work was done on
`claude/next-milestone-7r5wtq`, which the session that produced it was told to use. The story
ids are in every commit subject, where the rest of the working agreement puts them.

**One thing this milestone did not do to its own history.** The `--worktree` correction was
committed together with M5-S7's conversion rather than in its own pair of commits; the split
could not be made after the fact in the environment the work ran in. The code and its tests are
in that commit, and this line is here so a reviewer looking for them knows where they went.
