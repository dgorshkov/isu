# isu

`isu` is an issue tracker with no database. Issues are folders inside your repository, so the
pull request that fixes a bug also closes it, in the same diff — and status is *derived* from
what git already knows rather than stored and kept in sync by hand. One binary carries a CLI
and a TUI.

v1.0.0 deliberately ships no web UI, no bidirectional sync with other trackers, and no sprints,
points or burndown. Import from Jira is one-way and one-time. See PLAN.md for the full list.

## Install

```
go install github.com/dgorshkov/isu/cmd/isu@latest
```

`git` is a runtime requirement.
