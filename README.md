# isu

`isu` is an issue tracker with no database. Issues are folders inside your repository, so the
pull request that fixes a bug also closes it, in the same diff — and status is *derived* from
what git already knows rather than stored and kept in sync by hand. One binary carries a CLI, a
TUI and a local web UI.

## Install

```
go install github.com/dgorshkov/isu/cmd/isu@latest
```

`git` is a runtime requirement.
