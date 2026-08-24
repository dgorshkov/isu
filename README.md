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

## Status

Pre-release, under active construction. [`PLAN.md`](PLAN.md) is the build order and the
specification; it is worked top to bottom, one story per pull request.

## Not in v1.0.0

Deliberate omissions, so nobody has to ask:

- **isu Tower** — the hosted app for people without a clone. Separate repo, after v1.
- Cross-repo issues. Monorepo-first is a position, not an omission.
- A `fixed/` archive directory. Resolved issues stay in `issues/`.
- Sprints, story points, burndown, time tracking.
- Bidirectional sync with anything. Import is one-way and one-time by design.
- Notifications and email.

## Licence

Apache-2.0. See [`LICENSE`](LICENSE).
