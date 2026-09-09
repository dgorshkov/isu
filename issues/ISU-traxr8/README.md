---
schema: 1
id: ISU-traxr8
title: M7-S2 · Safe writes
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-2wyps1
blocked_by: ISU-2ws116
acceptance: every importer writes exclusively through this path, asserted by a grep test in
---
**Done** #15, 2026-09-03. One payload per attack, and the live one is the display name that is a
path: a comment file is named for its author, and an author's name is whatever they typed into
their profile. Everything outside a narrow alphabet becomes a hyphen before `SafeName` gets the
last word, so `../../../etc/passwd` becomes `etc-passwd` and lands in the folder it belongs in.

Every name is checked before anything is created, so a folder with one bad name in it writes
nothing rather than half of itself. **The symlink is the one `SafeName` cannot see** — every
element of that path is an ordinary name, and the escape is that one of them is already a link
somewhere else — so every element between the root and the file is stat'ed without following.

The grep test is an AST walk for `os.<write>` rather than a text grep, which is the same rule as
M2-S1's with one improvement: it cannot be fooled by the word appearing in a comment, and this
file's comments talk about writing constantly.

`Secret` formats as `<redacted>` through `fmt`, `%v`, `%s`, `%#v` and `encoding/json`, so the
ordinary way of building a message cannot leak one; `Redact` scrubs the messages isu did not
write, which is the half no type system reaches. **The deferral this story names held**: v1.0.0
fetches no attachment, so there is still no traffic for a decompression limit to sit on.
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
