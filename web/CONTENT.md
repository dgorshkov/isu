# isu — content plan and information architecture

<!-- title: isu — issues that branch, merge and review like code -->
<!-- tagline: Issues that branch, merge and review like code. -->
<!-- description: isu keeps issues as folders in your repository, so a bug report arrives as a pull request and the branch that fixes a bug carries that bug's state. Status is derived from git rather than stored anywhere. -->
<!-- lede: A command-line issue tracker whose issues are folders in your repository. The branch that fixes a bug carries that bug's state, so there is no second system to keep in step. -->
<!-- install: go install github.com/dgorshkov/isu/cmd/isu@latest -->

**This file is the site.** The landing page is built from the section sequence below and
carries no words of its own; `internal/site` reads this document, runs every command in it
against a repository it builds from scratch, and fails the build if what isu prints is not what
this file says it prints. A template that carried a sentence would be a second place the site's
copy lives, and two places drift the moment nobody is checking.

## The job of the page

An engineer arrives from a link, gives the page about thirty seconds, and decides one thing:
*is this a toy?* Everything below is ordered by how quickly it answers that.

They already have an issue tracker. They are not looking for a better one; they are looking for
a reason to believe this is not a weekend project with a landing page. So the page does not
argue. It shows the tool running, four times, and gets out of the way.

What has to be proved, in order:

| # | must prove | proved by |
|---|---|---|
| 1 | it exists and it runs | terminal output, above the fold |
| 2 | the central claim is real — status is derived, not stored | a board with six derived statuses on it |
| 3 | it is safe with more than one person | a claim, and what happens when two people want the same issue |
| 4 | it holds a team to something | a check failing on a branch, with a reason |
| 5 | it is built for agents as well as people | one JSON object per line |
| 6 | it knows what it is not | the out-of-scope page, linked, not hidden |

Nothing about roadmaps, nothing about philosophy, no testimonial, no logo wall. There is
nobody to quote yet, and a page that pretends otherwise fails the thirty-second test on the
first pretence.

## The page

### 1. Status is derived, never stored
<!-- id: derived -->
**Claim.** Every status on this board was computed, just now, from what trunk and the branches
say — there is no status field anywhere in the repository.
**Proof.** `isu board` against the sample repository.

An issue is a folder with a `README.md` in it. The file says `state: open`, `resolved` or
`dropped`, and nothing else about where the work has got to. Everything the board groups by is
derived: `done` because trunk says resolved, `in progress` because a branch says resolved where
trunk says open, `awaiting triage` because the folder exists on a branch and has never reached
trunk at all.

That last one is worth stopping on. `awaiting triage` is not a column somebody has to remember
to move a card out of. It is a fact about the repository — this issue has been reported and
nobody has merged it — and it stops being true the moment the report merges.

```console
$ isu board
main · 9 issues · remote refs 3h ago

done (2)
  APP-x4h7vb  p2  spike  What does a failed payment cost us?  dana
  APP-2mdz8k  p3  chore  Upgrade the linter                   sam

dropped (1)
  APP-8ptr5s  p2  chore  Rewrite the CSS in another framework  sam

awaiting triage (1)
  APP-63kqwn  p0  bug  Sign-up page 500s on Firefox  sam

in progress (1)
  APP-9cx2rt  p2  story  Show the sign-up queue on the board  priya · claimed by Priya Raman

open (4)
  APP-7f3akq  p1  bug    Login retries drop the second attempt   dana
  APP-40b1cc  p2  epic   Make sign-up reliable                   dana · 5 children
  APP-b5n3kt  p2  story  Send a receipt after the first invoice  priya
  APP-5wq7dn  p3  chore  Publish a status page                   sam
```

### 2. A claim is a branch, and the push is the lock
<!-- id: claims -->
**Claim.** Two people cannot both take the same issue, and nothing had to be locked to make
that true.
**Proof.** `isu show` on a claimed issue.

`isu claim` creates `isu/<id>`, writes `state: resolved` into the issue, commits with a random
`Isu-Claim:` trailer and pushes — before any work starts, so whoever loses the race has wasted
nothing. The push *is* the compare-and-swap: the trailer makes two claimants' commits different
objects, so the second push is not a fast-forward and git refuses it.

Saying `resolved` before the work is done is deliberate. A branch is a proposal, not a fact;
trunk is where state is true. Until the branch merges, the only thing that reads it is a board
that renders it as `in progress`, with the claimant and the age of the claim beside it.

```console
$ isu show APP-9cx2rt
APP-9cx2rt  Show the sign-up queue on the board

  status      in progress
  type        story
  state       open
  owner       priya
  created     2026-02-21
  priority    p2
  acceptance  the board shows queue depth and the oldest waiting account
  parent      APP-40b1cc  Make sign-up reliable (open)
  branches    refs/heads/isu/APP-9cx2rt
  trunk       main

  claimed by Priya Raman on refs/heads/isu/APP-9cx2rt, 2026-04-14T04:15:00Z

remote refs 3h ago
```

### 3. The rules read the diff, not your memory
<!-- id: checks -->
**Claim.** `isu check` fails a branch that says it fixed something and changed nothing else.
**Proof.** `isu check` on the claiming branch above, which is exactly that branch.

Nine rules run over the repository and over what this branch proposes. Some are about the tree
— a `parent` that names nothing, an epic with no children, an attachment over the limit — and
some are about the branch: an owner an agent reassigned, a resolution with no work beside it.

The branch above claimed an issue and has not done the work yet. In the file, that is
indistinguishable from a branch that resolved it: `state: resolved`, both times. What tells
them apart is the diff, so that is what the rule reads.

```console exit=1
$ isu check
main ← isu/APP-9cx2rt · 9 checks · 1 failure · remote refs 3h ago

fail  evidence  APP-9cx2rt  resolved on this branch, which changes nothing outside issues/: a claim writes `state: resolved` and touches nothing else, so this is a claim and not a resolution
```

### 4. Half of what reads this is an agent
<!-- id: json -->
**Claim.** Every command speaks newline-delimited JSON, and the shape is a contract rather
than a rendering.
**Proof.** `isu ready`, which prints JSON by default.

`isu ready` is the queue: the issues nothing is blocking, most urgent first, one JSON object
per line — so `isu ready | head -1` is the top of the queue and not an opening brace. JSON is
what it prints by default, because the caller that reads it most often is not a person;
`--json=false` is how you ask for the list a person reads. Every object carries the acceptance
criteria or the repro, because an agent picking work up needs the briefing before it needs
anything else.

Fields are added and never repurposed. What that shape means, field by field, is written down
and tested against the code that prints it.

```console
$ isu ready
{"id":"APP-7f3akq","title":"Login retries drop the second attempt","type":"bug","state":"open","status":"open","owner":"dana","created":"2026-02-21","priority":"p1","parent":"APP-40b1cc","blocked_by":[],"repro":"sign in, fail once, retry within 5s","acceptance":"","question":"","reason":"","resolution":"","on_trunk":true,"reopened":false,"contended":false,"stale":false,"claims":[],"elsewhere":[],"epic":null,"broken":null}
{"id":"APP-b5n3kt","title":"Send a receipt after the first invoice","type":"story","state":"open","status":"open","owner":"priya","created":"2026-03-05","priority":"p2","parent":"APP-40b1cc","blocked_by":[],"repro":"","acceptance":"a paid invoice sends one receipt, and only one","question":"","reason":"","resolution":"","on_trunk":true,"reopened":false,"contended":false,"stale":false,"claims":[],"elsewhere":[],"epic":null,"broken":null}
```

### 5. What it deliberately does not do
<!-- id: limits -->
**Claim.** There is no web UI, no sync, no sprints, and the import is one-way — and each of
those is a decision with a reason written down.
**Proof.** No terminal output. This section is a paragraph and a link, on purpose.

A tool that lists only what it can do is a tool you find the edges of in production. So
[what isu does not do](docs/not-doing.html) is a page of its own, linked from here rather than
buried: no `isu serve`, no bidirectional sync with anything, no story points, no cross-repo
issues, and sub-issue hierarchies recorded losslessly rather than flattened into a shape that
would misrepresent them.

### 6. Start here
<!-- id: start -->
**Claim.** Two commands, and the second one is the whole product.
**Proof.** No terminal output. The page has spent five cards earning this; the closer is a
place to go, not a sixth demonstration.

`go install github.com/dgorshkov/isu/cmd/isu@latest`, then `isu init --prefix APP` in a
repository you already have. [Getting started](docs/getting-started.html) takes it from there to
a merged fix, and every command on it was run against a scratch repository while this page was
built.

## The docs

Eight pages, in reading order. The order is the navigation: `docs/index.html` lists them in
this sequence and the masthead links to the first one, because a documentation index sorted
alphabetically is one where *getting started* is fourth.

| page | one job | who reads it |
|---|---|---|
| Getting started | from an empty repository to a merged fix | somebody evaluating isu |
| The data model | the folder, the frontmatter, the ids | somebody adopting it |
| Derived statuses | every status, and the ordered rule that produces it | somebody debugging a board |
| The check catalogue | the nine rules, what each refuses, and why | somebody whose CI is red |
| The JSON contract | every field of every payload | an agent, and whoever wrote it |
| Importing from GitHub Issues | the mapping, key by key | somebody migrating |
| Field notes | what running isu against real repositories found | somebody deciding |
| What isu does not do | the out-of-scope list, with reasons | somebody comparing |

Every command in every one of those pages runs during the build. Two of them —
*getting started* and nothing else — run against a repository that is created empty and then
written to, because a page that says `isu init` and never runs it is the page most likely to be
wrong.

The masthead carries three links and no more: **Getting started**, **Docs**, **Source**. A
navigation with eight entries on a site with nine pages is a table of contents pretending to be
a menu.

## The design

Decisions, not a mood board. Everything here is declared once in `web/assets/site.css` and
referenced by name; `internal/site/gates.go` fails the build on a hex value or a pixel type
size written anywhere else.

### Colour

Six tokens, named for their job. The contrast ratio of every pair the page actually puts
together is computed from these values at build time and held to WCAG AA — which is how
`--signal` on `--terminal` was caught at 3.18:1 and replaced with `--glow` at 9.09:1.

| token | light | dark | job |
|---|---|---|---|
| `--paper` | `#fbf9f5` | `#141312` | the page |
| `--ink` | `#12100e` | `#f2eee7` | body text and headings |
| `--slate` | `#5b5750` | `#a9a199` | secondary text |
| `--rule` | `#e2dcd1` | `#332f2a` | hairlines and borders |
| `--signal` | `#b4451f` | `#f0894f` | links, the caret, the one accent |
| `--terminal` | `#1b1917` | `#0c0b0a` | the ground of every proof card |

`--terminal-ink` and `--glow` are the two foregrounds that go on `--terminal`, and they do not
change between schemes: a terminal is dark in both, so making it lighter in dark mode would be
a decision about nothing.

### Type

**There is no webfont.** M8-S2 asks for fonts "self-hosted and subset, no third-party font
CDN"; the strongest available form of that is to ship no font at all. Prose is set in the
reader's own UI face and terminal output in their own monospace, so nothing is fetched, nothing
is subset, there is no swap on first paint, and the third-party-request gate passes because
there is nothing that could fail it.

| role | stack |
|---|---|
| prose | `ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, …` |
| wordmark, commands, terminal output | `ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, …` |

The scale is 1 rem, up and down by 1.25, expressed in rem so that a reader who has changed
their browser's text size gets what they asked for:

| token | size | used for |
|---|---|---|
| `--text-xs` | 0.8rem | the contents label |
| `--text-s` | 0.9rem | terminal output, tables, the foot |
| `--text-m` | 1rem | body |
| `--text-l` | 1.25rem | the lede, section claims, `h3` |
| `--text-xl` | 1.75rem | `h2` |
| `--text-2xl` | 2.4rem | `h1` |
| `--text-3xl` | 3.2rem | the hero |

### The signature element

**The proof card.** Every claim on this site sits directly above a dark card whose header bar
carries the command and whose body carries the bytes isu wrote — no highlighting, no
annotation, no invented prompt. The site is a stack of those cards, and that is the thing to
remember it by: it does not describe the tool, it runs it.

The card's one moving part is the caret after the install line, which blinks. It is the only
animation on the site and it stops for anybody whose system asks for reduced motion.
