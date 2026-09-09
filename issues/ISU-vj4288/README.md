---
schema: 1
id: ISU-vj4288
title: M2-S4 · The trunk history index
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-51kzyr
blocked_by: ISU-rctp3p
acceptance: `LoadHistory` answers reopen for a 5,000-issue repo without a git process per issue.
---
**Done** #6, 2026-08-26. **The first test case below was corrected as this story was
built.** An issue resolved and then reverted holds **three** states at trunk, not two: it
was open when it was created. The case beside it — an issue never resolved yields one entry
— is that same creation counted, and under any single rule the two numbers cannot both be
right. The rule that keeps the second is *one entry per trunk commit at which the value
changed, its first appearance included*, and collapsing the commits that changed something
else is what keeps the sequence about states rather than about typo fixes.
`--first-parent` is load-bearing and the story does not say so. A merge commit shows no
diff of its own, and under a pathspec git simplifies it away and reports the change at the
*branch* commit — a commit that was never on trunk, carrying the date the work was written
rather than the date it landed. The date recorded is the committer date for the same
reason. A blob that does not parse contributes no state and does not stop the walk, and
that is told apart from an epic — which parses and declares no state — by the lookup rather
than by the value, since both would read as the empty string.
**Branch** `isu/M2-S4-history-index`
**Why** `reopened` is one of the six statuses, and it is the only one that cannot be answered
from the current content of any ref. Walking history per issue inside the derivation package
would make M3-S1's "no git calls inside" rule a lie the moment M3-S4 lands. So the walk happens
here, in the git layer, and derivation stays pure over what it is handed.
**Build** `repo.LoadHistory(trunk) map[string][]StateAt` — for each issue, the ordered sequence
of `state` values its file has held at trunk, with the commit and date of each change. One pass
over `git log --format` plus one `cat-file --batch` fed object ids, in the style of M2-S2.
Never `%an`: the value comes from blob content.
**Tests first** an issue resolved then reverted yields two entries in order; an issue never
resolved yields one; an issue whose file was renamed is not followed (documented limitation,
asserted); a squash-only history yields the same sequence as a merge-commit history for the
same logical changes; process count stays constant as the number of issues grows.
**Done when** `LoadHistory` answers reopen for a 5,000-issue repo without a git process per
issue.
