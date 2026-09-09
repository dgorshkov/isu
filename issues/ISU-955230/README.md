---
schema: 1
id: ISU-955230
title: M6-S1 · Shell, layout and key map
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-ahaqb3
blocked_by: ISU-ams73p
acceptance: golden frames are stable across runs.
---
**Done** #12, 2026-09-02. `internal/ui` is a pure function of what it was handed, the way
`internal/model` is a pure function of what `internal/repo` loaded: the whole repository arrives
as an `Input` — the groups `isu board` renders, the derived board behind them, the freshness
sentence and the moment — and a frame is that plus which keys have been pressed. That layering is
what makes M6-S5's structural test easy to pass rather than something to arrange: a package that
cannot load anything cannot call git by accident.

Six lines of chrome whatever the terminal is, and **the message line is kept even when there is
nothing to say** — a footer that grows a line when an action reports moves the list up by one
under somebody's cursor, and the moment after `c` is the moment they are looking hardest. Every
line of a frame is trimmed on the right, which is not cosmetic: a pane padded to its width leaves
a run of spaces that is invisible on a terminal and very loud in a golden file.

**`-update` is not this package's flag.** The golden-file helper teatest brings registers one of
that name, and two flags of one name is a panic at init rather than a warning, so the golden
helper here reads it instead of declaring it.
**Branch** `isu/M6-S1-tui-shell`
**Build** bubbletea program: header with status counts, list pane, detail pane, footer key
hints. Resize-aware down to 80×24. Keys: `enter` open, `c` claim, `n` new, `r` ready queue,
`g` go to branch, `/` filter, `q` quit.
**Tests first** `teatest` golden frames at 80×24 and 140×40; a resize sequence; `q` quits
cleanly and restores the terminal.
**Done when** golden frames are stable across runs.
