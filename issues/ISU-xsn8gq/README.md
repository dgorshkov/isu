---
schema: 1
id: ISU-xsn8gq
title: M6-S5 · Actions from the TUI
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ahaqb3
blocked_by: ISU-g6psvy
acceptance: the TUI shares command implementations with the CLI, proven by a test that fails
---
**Done** #12, 2026-09-02. `claimIssue` and `report` are `isu claim` and `isu new` with the
command taken off the front, so the interface calls the function rather than something that agrees
with it. `checkout` is new and is deliberately not a claim: `g` on an issue nobody has claimed
from this clone is a sentence, because making a claim as a side effect of navigating to something
would claim work for whoever pressed a key by mistake.

`n` opens the editor on **a whole issue file rather than a form**. What comes back is parsed by
`internal/issue` and validated by the schema every other issue is held to, so there is no second
format to keep in step — and the id is allocated after the edit, because it is a hash of the
fields the editor is for. The terminal goes with it: `tea.Exec` releases the screen for as long as
the editor runs, and this package hands over a `Run()` rather than a process, which is how it
keeps the structural rule below.

**The proof M6-S5 asks for is stronger than the grep `internal/gitx` already runs.** That one says
nothing outside gitx builds a git command; this one says `internal/ui` does not import the git
binary or either of the two packages that reach it. A renderer that could run a git process would
run one per frame.

**Two things this milestone had to learn about terminals, and both were defects.** A burst of
printable characters arrives as one message carrying several runes — that is how a paste and a
fast typist both look — and every rune but the first was being dropped, so the interface lost keys
under exactly the condition somebody is going fast. And `q` pressed while an action is in flight
is now remembered rather than obeyed: leaving in the middle of a push would abandon the one
operation in this product that has to be atomic.

**teatest does not survive `tea.Exec`, and that is worth writing down.** Measured at about one run
in two, the program released the terminal and never repainted — and every run once the test waited
for a frame before typing. The model was right each time; what was wrong was synchronising on an
intermediate byte stream. The action tests run the same program over an ordinary pair of buffers,
which is what `isu ui` itself runs over, and wait on the fake rather than on the screen.
**Branch** `isu/M6-S5-tui-actions`
**Build** `c` claims through the same code path as `isu claim`; `n` opens an editor for a new
issue; `g` checks out the claiming branch. Every action reports its result inline and never
fails silently.
**Tests first** claiming an already-claimed issue shows the holder; the editor is injected and
tested with a fake; `g` on an unclaimed issue is a no-op with a message.
**Done when** the TUI shares command implementations with the CLI, proven by a test that fails
if the TUI package calls git directly.

---
