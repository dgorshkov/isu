---
schema: 1
id: ISU-ahaqb3
title: M6 · TUI
type: epic
owner: dmitry
created: 2026-09-01
priority: p2
---
**Status** done — all five stories landed in one pull request. the working agreement's grouping rule allows it: they
are neighbours in this document, they belong to one milestone, and they share one subject a
reviewer can hold at once — the branch would have been `isu/M6-S1-S5-tui`, and this work was done
on `claude/next-milestone-gzyx59`, which the session that produced it was told to use. The story
ids are in every commit subject, where the rest of the working agreement puts them.

**`bubbles` is in the allowlist and is not used.** The list, the filter and the detail pane's
scroll are a handful of statements each over state this package already holds, and a component
that brings its own key map and its own styles would have been more to reconcile than to write —
the filter alone would have had to have its blinking cursor turned off to keep a golden frame
stable. bubbletea, lipgloss and glamour are all used, and teatest is used for what M6-S1 asks it
for. Removing a dependency the allowlist permits is not adding one, and go.mod says so.

**Two keys the map in M6-S1 does not name.** `h`/`←` folds an epic and `l`/`→` unfolds it, because
M6-S3's "navigating across a collapsed epic" requires a way to collapse one and this document
names none; and `esc` clears a filter, closes the filter line, and gives the keys back from the
detail pane. `q` quits from everywhere, including the pane `enter` opened — a key that quits from
one pane and does something else from another is the one thing a person has to keep in their head.

**`isu ui --json` prints `isu board`'s payload**, and that is a decision this document does not
make. Every command supports `--json` and `contract_test.go` enforces it, but an interface is not
a thing an agent can read; refusing to answer would leave one guessing, and inventing a second
shape would be a contract nobody asked for. What the interface would open on is the board.
