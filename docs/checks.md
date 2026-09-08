# The check catalogue

`isu check` runs isu's rules over the repository and over what your branch proposes. A failure
exits 1 and a warning does not: two people about to do the same work is worth saying and is not
a reason to refuse a pull request.

## Two kinds of rule

A **tree** rule is about the repository as it stands — a parent that names nothing, an epic with
no children — and needs no branch. A **branch** rule is about what this branch proposes: a
resolution with no code beside it, an owner an agent reassigned. `--scope` picks one kind.

The pre-commit hook `isu init --hooks` writes runs the tree rules only, because at pre-commit
time the change being committed is not a commit yet and the branch rules would be answering
from the commits already there. It also passes `--worktree`, which reads the issues on disk
rather than at a ref: a check that read a ref before a commit would be answering about the
commit *before* the one being made, and would refuse the very commit that fixed what it was
complaining about.

## The nine rules

| rule | scope | what it refuses |
|---|---|---|
| `attachments` | tree | an attachment bigger than `attachment_max_bytes` |
| `claims` | tree | two branches claiming one issue, and a claim that has gone quiet |
| `cycles` | tree | an epic that is its own ancestor, and anything waiting on itself in a ring |
| `duplicates` | tree | two branches reporting different issues under one id |
| `epics` | tree | an epic with no children |
| `evidence` | branch | resolving something with no work beside it, and a drop with no reason |
| `links` | tree | a `parent` or `blocked_by` naming an issue that does not exist, or a parent that is not an epic |
| `owner` | branch | a commit by a configured agent reassigning an issue's owner |
| `schema` | tree | an issue file that will not parse, fails the schema, or sits in a folder not named after its id |

A clean repository says so and exits 0:

```console
$ isu check --scope tree
main ← isu/APP-9cx2rt · 7 checks · nothing to report · remote refs 3h ago
```

## Evidence is the rule that makes a claim a claim

`isu claim` writes `state: resolved` on the claiming branch *before* any work starts — that is
the whole compare-and-swap, and it is why a claimed issue reads `in progress`. So a branch that
claimed something and did nothing looks, in the file, exactly like a branch that resolved it.
What separates them is not the field, which is identical, but the diff beside it.

```console exit=1
$ isu check
main ← isu/APP-9cx2rt · 9 checks · 1 failure · remote refs 3h ago

fail  evidence  APP-9cx2rt  resolved on this branch, which changes nothing outside issues/: a claim writes `state: resolved` and touches nothing else, so this is a claim and not a resolution
```

What counts as work depends on the type. A bug, a story or a chore needs a change outside
`issues/`. A spike needs a file in the issue's own folder that is not `README.md`, because a
spike is answered rather than fixed and the artifact is the answer. A drop needs a `reason` and
a `resolution` from the enum.

The rule reads the branch and not the board, so an issue no commit on the branch touched is not
its business. A pull request is answerable for what it changed.

## Owner is expensive to change on purpose

`owner` is the accountable human, set at triage. A commit whose author is listed under `agents`
in `.isu.yml` and which changes an issue's `owner` is a failure — an agent may do the work, but
it may not decide who answers for it.

## Running them

```sh
isu check                      # everything, against the current branch
isu check --scope tree         # the repository only, no branch
isu check --worktree           # the issues on disk, uncommitted edits included
isu check --json               # one finding per line
isu check --ref origin/main    # say what trunk is, which CI has to
```

That last one matters in a pipeline. A pull request build checks out a merge commit and no
branch, so without `--ref` isu compares the repository against itself and every rule about the
branch passes silently. The workflow `isu init --actions` writes already does this.

## Adding one

A check is one file and one registry line. It implements `Name`, `Scope`, `Describe` and `Run`
over an input the CLI assembled, it runs no git of its own, and it never fails — a check that
cannot answer has found nothing. `isu check --help` is generated from the registry, so a rule
that exists is a rule that is documented.
