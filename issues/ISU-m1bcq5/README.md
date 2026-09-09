---
schema: 1
id: ISU-m1bcq5
title: M8-S2 · Landing page
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-dj6g6k
blocked_by: ISU-he1wf1
acceptance: every artifact on the page came out of the binary in this repo.
---
**Done** #21, 2026-09-09. Seven sections, five of them a claim above a card containing bytes isu
wrote. The card is the signature element: a header bar with the command and a body with the
output, and nothing in between for a designer to embellish.

**"Fonts self-hosted and subset" was answered by shipping no font at all**, and that is the one
place this story's brief was met differently from how it was written. The strongest available
form of "no third-party font CDN" is to have nothing to fetch: prose is set in the reader's own
UI face and terminal output in their own monospace, so there is no subsetting step, no swap on
first paint, and the third-party-request gate passes because there is nothing that could fail
it. If a brand face is wanted later it is a stylesheet change and a file, not a redesign.

**The contrast gate earned its place on the day it was written.** `--signal` on `--terminal` is
3.18:1 and looks perfectly fine; the terminal card's command line uses `--glow` at 9.09:1
because a computed number said so and an eye did not.

**The page was read back three times by a hostile reader, and each pass found something the
gates could not.** They are recorded here because the pattern is the point: every round turned
up at least one place where the site *said* something that was not mechanically true, on a site
whose whole argument is that it never does.

- Round one: section 5 said the out-of-scope page "is linked from here" and carried no anchor;
  a `sh` block with placeholder ids in it was borrowing the dark ground that means "the binary
  wrote these bytes"; the copy recommended `isu ready --json | head -1` beside a card proving
  the flag is a no-op; and the type scale's comment contradicted its own six ratios. `gateProse`
  and the `ran`/`sketch` grounds came out of it.
- Round two: twelve scrolling regions across the site and not one `tabindex` — WCAG 2.1.1, in
  the element this site is built around. `gateFocus` reads the stylesheet rather than a list of
  element names, and **a scrolling selector it cannot evaluate fails the build**, because the
  bug was not the missing rule but that the gate did not know what it was not checking.
- Round three: the share card was 1200×630 of dark ground with a logo on it and no argument,
  which is the one asset that reaches a reader before the page does. The page's only call to
  action was a command with two `aria-hidden` spans round it, so selecting and pasting it gave
  `$ go install …@latest_` — `aria-hidden` hides a string from a screen reader and not from a
  clipboard. Four of six sections named a documentation page and did not link it. And the middle
  verb of the headline was the one the page never demonstrated.

What each of those produced is in the diff: a card that sets the tagline out of `CONTENT.md`, a
prompt and caret drawn by the stylesheet with `gateDecoration` refusing text inside anything
`aria-hidden`, five links where there were two, and a section that shows the loop closing rather
than four snapshots of a state machine.

**A fourth read found the one place the front page and this repository's own evidence
disagreed.** Section 2 said "two people cannot both take the same issue" while
`docs/field-notes.md` says, in as many words, that the board reads local refs only and a
colleague's claim is therefore invisible. Both are true — the *lock* holds, because the second
push is refused; the *warning* does not — and the landing page was making the multiplayer claim
without the carve-out, on the one page of eight it did not link. The carve-out is in section 2
now, with the link, which is the move section 5 already makes with `not-doing` and gets credit
for.

**A section carries a sequence of samples rather than one.** `Section.Samples` is a slice because
a state machine is not proved by a snapshot of it: the merge is now the same command on two
claims one merge apart, `in progress` and then `done`, with no flag on either. Getting there
needed a fixture that contains a completed loop, which it did not — every issue in it was either
finished before the samples start or still in flight — so `APP-b5n3kt` is claimed on a branch and
then squashed onto trunk, and the branch is left standing because that is what happens to
branches.

**`--ref <an ancestor of trunk>` is not a way to show a merge, and that is a product
observation.** Reading the fixture at `main~1` reports `in progress (contended)` with two
claimants, because `refs/heads/main` is a ref that proposes `resolved` and the contention rule
counts it. It is correct and it is unreadable on a landing page, which is why the two cards are
flagless. M3's contention rule deciding that trunk-ahead-of-the-ref is not a rival claimant is a
story of its own.

**The tutorial cannot quote a claim, and now says so instead of ending in a sketch nobody
explains.** `isu claim` writes a random `Isu-Claim:` nonce, so the commit id differs every run;
the board after it reads `remote refs just now`; and the claimant it prints is whoever `git` is
configured as on the machine that built the page — `claimed by Claude`, on the run that found
this. A page whose bytes must be identical on every machine can quote none of those three, so
`docs/getting-started.md` names the reason and sends the reader to the front page, where the
clock and the identities are fixed. Its lede promised "a merged fix" and now promises what it
delivers.

**Accessibility, twice over.** The fix that made every scrolling box reachable gave each one
`role="region"`, which makes it a *landmark*: `docs/json.html` announced sixteen landmarks all
called "table". They are `role="group"` now — reachable, labelled on entry, out of the landmark
list — and a block that did not run says "example, not run" rather than "terminal output", which
is the distinction the stylesheet had been drawing since the round before and the accessibility
tree had never heard of. And the five cards on the landing page were reachable while announcing
nothing on focus: no rule covered `.proof pre`, so it fell back to the browser's outline, which
`.proof`'s own `overflow: hidden` clipped on three sides. The ring is inset now, in `--glow`,
which is 9.09:1 on the terminal ground.

**The install line was 40% off the right-hand edge of a phone.** `gateWidth` could not see it:
that gate holds `<pre>` and `<table>` to a scrolling box, and the install line is a `<p>`, so
"no page-level horizontal scroll" and "readable on a phone" came apart exactly where the page
asks somebody to type something. It wraps under 30rem and `user-select: all` makes one tap take
the command and neither pseudo-element — verified in Chromium, along with the focus ring and the
landmark counts.

**The share card is 1200×630 and had three letters on it.** It sets the tagline now, read out of
`CONTENT.md` so the card and the page cannot disagree, which cost the bitmap face an alphabet —
five by nine, the last two rows for descenders, and a tagline carrying a character the face
cannot set fails the build rather than drawing a hole.

**And `TokensAgree` exists because this document lied about itself twice.** The plan's colour and
type tables are prose about a stylesheet, and prose about a file stops being true: the type table
went on saying `--text-l` was `1.25rem` for a week after it became a clamp. Every row naming a
token is now held to what the stylesheet declares, in both schemes.

**Two findings were not acted on, and the reason is the same in both cases: they are somebody
else's story.**

- **`isu board` prints `done` and `dropped` first.** `internal/cli/view.go` renders the groups in
  `model.Statuses` order, and that slice is the *precedence* table from the data model — which rule wins when
  two match. That has nothing to do with what a person wants to read first, so the flagship card
  on the landing page opens with three rows of finished work, one of them a joke about rewriting
  the CSS. It is a real defect and it is M2/M3's, not M8's; a display order of `open`,
  `in progress`, `awaiting triage`, `reopened`, then the terminal groups is the obvious fix and
  it moves golden files in `internal/cli` that this pull request has no business moving.
- **The `isu ready` card is two 22-field objects with `"question":"","reason":"","resolution":""`
  visible in both.** The complaint is fair — it reads as "this JSON is mostly empty" — but two
  lines is what makes *newline-delimited* legible, and `isu ready` has no flag that would print
  one. Filling those fields in the fixture would mean inventing content for states the issues are
  not in, which is the one thing this site may not do.

`SiteURL` is the only absolute URL the site contains. It was
`https://dgorshkov.github.io/isu` while this story was written and is
`https://isu-website.netlify.app` now, decided under M8-S3. Everything else is relative, so the
site works from a `file://` checkout, from a deploy preview at a URL nobody chose and from a
domain of its own without being rebuilt — changing the host is one constant and a `make site`.
**Branch** `isu/M8-S2-landing`
**Build** the page from `CONTENT.md`. Colours and type sizes come from CSS custom properties
declared once; fonts are self-hosted and subset, no third-party font CDN. Terminal output, board
renders and check results are produced by running `isu` against a fixture repo at build time and
embedded — never authored by hand.
**Tests first** a test regenerating every sample and failing if the committed page differs —
this is what stops the site drifting from the product; a test asserting no template hardcodes a
hex value or a pixel font size; a test asserting the built HTML makes zero third-party network
requests.
**Done when** every artifact on the page came out of the binary in this repo.
