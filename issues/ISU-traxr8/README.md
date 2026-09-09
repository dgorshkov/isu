---
schema: 1
id: ISU-traxr8
title: M7-S2 · Safe writes
type: story
state: open
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-2wyps1
blocked_by: ISU-2ws116
acceptance: every importer writes exclusively through this path, asserted by a grep test in
---
**Branch** `isu/M7-S2-safe-writes`
**Why** An importer writes attacker-influenced data into your repository. Ticket titles,
comment bodies, author display names and custom field values all originate outside your
control, and on a public repository "outside your control" means anyone with a browser.
v1.0.0 never serves that data over HTTP — the web UI is out of scope, and `glamour` renders to
a terminal — so **writing it to disk is the entire attack surface, and it gets its own story.**
**Build** one guarded write path that every importer must use. Filenames sanitised and
constrained to the issue's own folder — reject `..`, absolute paths, symlinks, control
characters, reserved Windows names, and names over 255 bytes. Per-file and per-issue size
caps. API tokens read from the environment or a credential helper, never from a flag, never
written to disk, and redacted from every log line and error string.
**The decompressed-size limit and the content-type sniffing are deferred along with the
downloads that needed them.** v1.0.0's one importer records attachment links and fetches
nothing (M7-S5), so a zip-bomb guard here would be a defence with no traffic on it and a 99%
coverage floor to answer to. They return with the first importer that fetches a file, and the
guard they belong behind is this one.
**Tests first** one payload per attack: `../../etc/passwd`, an absolute path, a symlink, a
4,000-character filename, a filename containing a newline, **a comment author whose display
name is a path** — the comment file is named for the author, so this is the live one — an issue
body over the per-issue cap, and a token interpolated into an error message.
**Done when** every importer writes exclusively through this path, asserted by a grep test in
the same style as M2-S1.
