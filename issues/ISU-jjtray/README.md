---
schema: 1
id: ISU-jjtray
title: M4-S6 · `isu comment`
type: story
state: resolved
owner: dmitry
created: 2026-08-23
priority: p2
parent: ISU-6mkqq5
blocked_by: ISU-kww9p8
acceptance: commenting is one command and the result renders in `isu show`.
---
**Done** #9, 2026-08-31. **This is the story that amended M2-S1's grep**, as recorded at the head of
this milestone. Beside that, one thing changed a layer up: `isu show` now lists an issue's folder
by walking it rather than through `issue.Load`, which decodes the README on the way past. What is
beside an issue is not a fact about the issue, and a half-written README must not take its
attachments and its comments off the screen — the same choice §2 made in the loader, arriving in
the renderer.
**Branch** `isu/M4-S6-comment`
**Why** `comments/` has been in the data model since the data model and is rendered by the TUI, but
nothing in the plan ever wrote one. A tracker whose only write path is hand-authoring a file is
not a tracker.
**Build** `isu comment <ID> -m` and `$EDITOR`, appending `comments/<date>-<author>-<nn>.md`
with the sequence chosen by looking at what is already there.
**Tests first** two comments by the same author on the same day produce `-01` and `-02` and
neither overwrites the other; a comment containing `---` at the start of a line does not
corrupt anything downstream; a comment on a nonexistent issue is refused; the issue's
`README.md` is untouched, asserted with `git diff --exit-code`.
**Done when** commenting is one command and the result renders in `isu show`.
