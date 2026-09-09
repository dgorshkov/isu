---
schema: 1
id: ISU-he1wf1
title: M8-S1 · Content plan and information architecture
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-dj6g6k
blocked_by: ISU-2wyps1
acceptance: someone who has never seen the project can read `CONTENT.md` and say in one
---
**Done** #21, 2026-09-09. `web/CONTENT.md` is not a brief the page was built *from* — it is the
page. `internal/site/content.go` parses the section sequence, the claim, the copy and the sample
out of it, and the template carries structure and not one word, because a template with a
sentence in it is a second place the site's copy lives.

**The executable-documentation harness is this story's, and M8-S3 reuses it.** A ```console
fence is run; a `$ ` prompt anywhere else — loose in the prose, or in an `sh` fence — fails the
extraction, because a transcript nothing runs is exactly the thing that rots. A command must
begin with `isu`, so a document cannot ask the harness to run something else.

**The build found one defect in `isu init`, and it is not fixed here.** `reportWrite` falls back
to the path list for its headline when there is no issue id, and then prints the same list
underneath — so `isu init` names its three files twice. It is on the site, in
`docs/getting-started.md`, exactly as the command prints it. Fixing it changes M4's output and
its golden files, which is M9-S3's business or a story of its own, not a website's.
**Branch** `isu/M8-S1-content-plan`
**Why** The page has one job: an engineer decides in thirty seconds whether this is a toy.
Decide what must be proved, and in what order, before anything is designed.
**Build** `web/CONTENT.md` — the section sequence, the single claim each section makes, the
artifact that proves it, and the finished copy. Plus the docs tree and the navigation. Then the
design decisions, in the same file and kept short: four to six named colours with hex, the
typefaces and their roles, the type scale, and the one signature element the site will be
remembered by. No markup in this story.
**Tests first** every command and code sample in `CONTENT.md` executes against a scratch repo
and produces the output the document claims.
**Done when** someone who has never seen the project can read `CONTENT.md` and say in one
sentence what isu does and who it is for.
