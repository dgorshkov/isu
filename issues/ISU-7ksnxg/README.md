---
schema: 1
id: ISU-7ksnxg
title: M7-S5 · GitHub: comments, fields and closing pull requests
type: story
state: resolved
owner: dmitry
created: 2026-09-01
priority: p2
parent: ISU-2wyps1
blocked_by: ISU-djfwz2
acceptance: a realistic export imports completely and idempotently — twice gives a zero-length diff.
---
**Done** #15, 2026-09-03. **Comments cost pages, not issues, and that was worth finding.**
`/repos/{owner}/{repo}/issues/comments` lists every comment in the repository, so the thing that
looks like it needs a request per issue does not — they are grouped by the `issue_url` each one
carries. Issue field values are the one genuine exception, because there is no repository-wide
list of them; they are read per issue as this story specifies, `--fields=false` turns them off,
and the dry run reports what the budget cost either way.

**A field value is not shaped the way this document assumed, and the difference is silent.** The
API spells the name `issue_field_name`, not `name`, and a select field's answer is in
`single_select_option` or `multi_select_options` rather than in `value` — so a reader looking for
`name` and `value` would have produced a `source.yml` keyed by empty strings with every select
field blank, and nothing would have said so. Both are read, the shorter spellings a hand-written
dump uses are accepted beside them, and a select keeps its option's name while dropping the id
and the colour, which are GitHub's rather than this repository's.

Attachments are recorded and not downloaded, as specified, and the note saying so is
unconditional: a reader has to be told that resolving one still needs github.com whether or not
this repository has any.

**One decision this document did not make: two imports colliding are told apart by the source
*ref*, not the key.** This story asks that running one import twice produce a zero-length diff
and M7-S4 asks that two imports into one tracker be refused rather than overwritten. Both are
true only if "is this folder already this issue" is asked of `acme/widgets#7` rather than of
`#7` — a key is repository-relative, so the pair the rule exists to catch is exactly the pair
that would look identical. A folder isu wrote itself has no `source.yml` at all and is never
overwritten either.

The command surface is here rather than in M7-S1 because this is the commit that made the whole
pipeline usable, and M7-S1's own done-when — a dry run by default, `--write` to change that — is
asserted by the tests in this pair. `isu import` takes the source as an argument rather than
offering a subcommand per tracker, since everything after "which tracker" is identical.
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
