---
schema: 1
id: ISU-fm1qj4
title: M3-S5 · Squash-merge lifecycle
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-stb9h3
blocked_by: ISU-7w8xhg
acceptance: the squash lifecycle test passes without changing M3-S1..S4. ---
---
**Done** #8, 2026-08-27. The lifecycle passes without changing M3-S1..S4, which is the result
the story was written to get. **"Build nothing new" is wrong by one function**, and the story's
own test list is what makes it wrong: it asks for a trailer recovery and a subject fallback,
and neither existed. `model.Resolves` is that function, and it reads two of the data model's three
tiers — the two that are in a commit message. The middle tier, the branch name the merge
recorded, is not in one: it needs the merge's own refs, so it belongs to whatever loads them,
and M7-S3 already scans for it.

**The subject tier is anchored on the repository's `prefix`, and the anchor is the whole of its
safety.** An id is letters, digits, a hyphen, an underscore and a full stop — because an
imported issue keeps its source key verbatim — so every word of an English sentence is a legal
id, and an unanchored scan would return the first word of every squash commit in the repository
with total confidence. An empty prefix therefore reads no subjects at all: nothing downstream
can tell a wrong link from a right one, so "which issue is this about" has to answer nothing
rather than guess. A trailing full stop is trimmed before the check, since a subject that ends
in one is ordinary English and an id that ends in one is not something isu generates. A run of
two or more stops ends an id wherever it appears, because `..` is no part of any key; a single
interior one is left alone, because `ISU-1.2` is a key somebody could have imported and
truncating it would turn a right link into a wrong one.

**A revert is the one commit whose subject means the opposite of what it says**, and the
subject tier skips it — `git revert` quotes the subject it undid verbatim, so the anchor is
present and points backwards, and M7-S3 would record the commit that un-resolved an issue as
one that resolved it. `Reapply "…"` is skipped with it. The trailer tier is deliberately *not*
guarded: git writes the body of a revert itself and does not carry the original trailers over,
so an `Isu-Resolves:` on one was put there by somebody who meant it.

**How a trailer's value is read cost two attempts, and the second is the rule.** Splitting on a
comma alone is isu's own habit mistaken for a convention — git says nothing about what goes
inside a trailer value — so `Isu-Resolves: ISU-7f3akq ISU-39ka2p` named nothing, fell through
to the subject tier, and resolved whatever the forge had put there instead. The first fix
demanded every token look like a key, which broke the ids that do not: `4821` from a GitHub
import and `ISU_7f3akq` are ids, `ValidID` admits them on purpose, and dropping them
reintroduced the same wrong link through another door. So the rule depends on how many things
are in the value: a comma-separated element is judged only by `ValidID`, and an element holding
several words must be all key-shaped, because `the login one` is three legal ids and reading
the value as one token was the only thing filtering prose out. A value folded across indented
lines is read whole, per git's own grammar, rather than losing every claim after the first.
**Branch** `isu/M3-S5-squash`
**Build** nothing new. This story exists to prove the model survives the merge strategy most
teams use.
**Tests first** an end-to-end lifecycle using **only** squash merges: report → triage →
claim → fix → merge → revert, asserting the derived status at every step. Then assert the issue
id is recoverable from the trunk commit that set `resolved` **via the `Isu-Resolves:` trailer**,
and separately that the subject-parsing fallback recovers it from a squash subject that carries
the id, and returns nothing rather than a wrong answer on one that is only a pull request
title.
**Done when** the squash lifecycle test passes without changing M3-S1..S4.

---
