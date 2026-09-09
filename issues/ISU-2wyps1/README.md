---
schema: 1
id: ISU-2wyps1
title: M7 · Importers
type: epic
owner: dmitry
created: 2026-09-01
priority: p2
---
**The importer is GitHub Issues, and it used to be Jira.** The swap is less a change of target
than a change of what the source can be asked. Somebody adopting isu is already in a git
repository, that repository is overwhelmingly on GitHub, and the issues they want out are
therefore sitting beside the code they are migrating — no export request, no admin, no licence.
GitHub also answers for free two questions Jira answers through an integration somebody had to
install: **which pull request closed this issue**, and **what is this issue blocked by**. And it
can be read without credentials, which is what lets these stories be tested against real public
repositories rather than against a fixture nobody can check. Jira joins Linear on the
out-of-scope list as a deferral, and M7-S1's source interface is the seam it comes back through.

**Four things GitHub Issues can now do did not exist when this milestone was first written**,
and each is read where present rather than assumed: **issue types** (organisation-level, at most
twenty-five, `type` on the issue itself, defaulting to Task/Bug/Feature); **sub-issues** (100
children per parent, eight levels of nesting, and they may cross repositories); **issue
dependencies** — `blocked_by` and `blocking`, August 2025 — which is isu's `blocked_by` under
its own name; and **issue fields**, structured custom metadata that reached general availability
in July 2026 and is precisely the two hundred custom fields M7-S1 refuses to let into
frontmatter. A repository with none of them still imports, and the dry run says which it found.

**Ids are the one place the data model had to be amended, and the data model carries the amendment.**
A Jira key is already a legal id; a GitHub key is `#1234`, which is repo-scoped, shares its
sequence with pull requests, and is not a folder name. `<PREFIX>-1234` it is.
