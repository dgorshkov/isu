---
schema: 1
id: ISU-7ksnxg
title: M7-S5 · GitHub: comments, fields and closing pull requests
type: story
state: open
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-2wyps1
blocked_by: ISU-djfwz2
acceptance: a realistic export imports completely and idempotently — twice gives a zero-length diff.
---
**Branch** `isu/M7-S5-github-content`
**Build** comments become files under `comments/`, named by date, author and sequence per
the data model — an active issue routinely has three comments from the same person on the same day,
and without the sequence the importer would silently keep only the last. **Issue field values
join everything else unmapped in `source.yml`**: they are read per issue from
`/repos/{owner}/{repo}/issues/{number}/issue-field-values`, they are the case M7-S1 wrote its
rule for, and a field an organisation invents next year must not need a schema bump here.
**Attachments are recorded rather than downloaded, and that is a finding rather than a
preference.** A GitHub attachment is a `github.com/user-attachments/assets/…` link inside the
markdown. On a private repository it cannot be fetched with a personal access token or a GitHub
App token at all: the asset wants a browser session, and the only programmatic route is to
re-request the issue with `Accept: application/vnd.github.full+json` and race a JWT out of
`body_html` before it expires minutes later. An importer whose completeness depends on winning
that race is one that half-works on exactly the repositories people most want migrated. So
v1.0.0 rewrites nothing: the links stay in the body byte for byte, they are listed in
`source.yml`, and the dry run says how many there are and that resolving them still needs
github.com. This is why M7-S2 defers its decompression limit, and why this story is no longer
the largest in this milestone.
**Closing pull requests are the strongest evidence tier there is, and here they are free.**
GitHub already stores which pull request closed an issue — `closedByPullRequestsReferences`,
and the `closed` timeline event's `commit_id` where a commit closed one directly — and both
arrive with the issue rather than through an integration somebody had to install. They outrank
every tier of M7-S3's scan, which is what that recorder was built to arbitrate.
**Tests first** against a recorded transcript, never the live API: a comment containing
frontmatter delimiters does not corrupt the issue file; three comments by one author on one day
produce three files rather than one; forty custom field values produce clean frontmatter and a
complete `source.yml`; an attachment link survives the body byte for byte and is listed;
`closedByPullRequestsReferences` outranks the M7-S3 scan when both have a link for the same
issue.
**Done when** a realistic export imports completely and idempotently — running it twice
produces a zero-length diff.

---
