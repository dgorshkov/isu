# What isu does not do

A tool that lists only what it can do is a tool you find the edges of in production. This is the
list, with the reason beside each one, so nobody has to ask.

## No web UI

Version 1.0.0 is a command line and a terminal interface. There is no `isu serve`.

Dropping it also drops the markdown-to-HTML pipeline, the HTML sanitiser and the XSS surface
that came with them. Issue bodies arrive from importers and from strangers; `glamour` renders
them to a terminal, where a `<script>` tag is just text. That is a smaller thing to get right
than a sanitiser, and it is right by construction rather than by review.

## No Jira or Linear import

One importer, done properly, beats three done at once. Whoever is adopting isu is already in a
git repository, and that repository is almost always on GitHub — which answers, for free and
without an administrator, the two questions Jira answers only through an integration somebody
installed. The [importer's](importing.html) source interface is the seam the next one arrives
through.

## No bidirectional sync with anything

Import is one-way and one-time. Two systems that both believe they own the state are two
systems that disagree at three in the morning, and the whole premise of isu is that there is one
place the state lives and git already knows how to merge it.

## No sprints, points, burndown or time tracking

None of these is a fact about the repository. All of them are a fact about a process, they
change when the process does, and a tracker that models them ties your issue data to this
quarter's methodology.

## No cross-repo issues

Monorepo-first is a position rather than an omission — and it is what keeps imported
`<PREFIX>-<number>` ids from colliding, since two repositories' `#1` never meet.

## No sub-issue hierarchies

isu has one `parent`, it must name an epic, and an import spends it on the milestone. A GitHub
tree eight levels deep is recorded whole in `source.yml` rather than flattened into a shape that
would misrepresent it, which makes modelling it later a story rather than a re-import.

This is the largest thing version 1.0.0 knowingly declines to model.

## No downloaded attachments

GitHub's assets want a browser session on a private repository, so an import records the links
and leaves them resolvable where they already live. An importer whose completeness depends on
winning a race against an expiring token is one that half-works on exactly the repositories
people most want migrated.

## No archive directory

Resolved issues stay in `issues/`. There is no `fixed/` and no archive command: state lives in
the file, so moving the file would be storing the state twice and leaving somebody to keep the
two in step.

## No renumbering

Ids are permanent from creation. There is no `isu renumber`, because `blocked_by` on one branch
has to keep pointing at the right issue while a hundred other branches are in flight.

## No notifications, and no email

isu reads a repository and prints. Where you want to be told about a change is your forge's
question, and it already has an answer.

## No second forge

This project runs on GitHub. A pipeline for a forge nobody here uses is a pipeline that rots
while claiming to be a second opinion. Nothing in the design needs one: the squash-merge
evidence tiers stand on GitHub's own squash settings, so adding another later is work rather
than a redesign.

## Deferrals, not rejections

The web UI, the other importers and the WebAssembly derivation demo are deferrals. Each has a
milestone's worth of design already written down, and each was cut because version 1.0.0 ships
sooner without it — not because it is a bad idea.
