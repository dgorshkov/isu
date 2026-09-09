# isu

`isu` keeps issues as folders inside your repository, so they branch, merge and review like the
code that fixes them: a report arrives as a pull request, the branch that fixes a bug carries
that bug's state, and merging is what makes it true. Status is *derived* from what git already
knows rather than stored and kept in sync by hand. One binary carries a CLI and a TUI.

v1.0.0 deliberately ships no web UI, no bidirectional sync with other trackers, and no sprints,
points or burndown. Import from GitHub Issues is one-way and one-time. See `docs/not-doing.md`
for the full list.

## Install

```
go install github.com/dgorshkov/isu/cmd/isu@latest
```

`git` is a runtime requirement.

## Documentation

`docs/` holds getting started, the data model, every derived status with its rule, the check
catalogue, the JSON contract, the GitHub Issues mapping, what isu deliberately does not do, the
design record and the field notes. `web/CONTENT.md` is the landing page. Every command in either
runs during the build, against a repository the build creates, and the site is regenerated with
`make site`.

## How isu is built

`issues/` is this project's own build record, kept in isu: ten milestones as epics, fifty-one
stories beneath them, each carrying its brief and — once it has landed — the pull request that
landed it. `isu board` is where the build is and `isu ready` is what comes next. The working
agreement is in `CLAUDE.md` and the design record is `docs/design.md`.
