# Getting started

From an empty repository to a claimed issue, with nothing installed but `isu` and the `git` it
shells out to. Every `$ ` command on this page is run against a repository the site's build
creates from scratch, and the lines under it are what it printed.

## Install

```sh
go install github.com/dgorshkov/isu/cmd/isu@latest
```

`git` is a hard runtime requirement. isu never reimplements it: your git configuration, your
hooks, your credential helper and your LFS filters all apply, because every read and every
write goes through the binary you already have.

## Set the repository up

`isu init` writes `.isu.yml` at the top of the repository. `prefix` is the first half of every
issue id — `APP-7f3akq` — and there is no default, because the half of an id that says which
project this is has no sensible guess. Nothing else is written, since a configuration file full
of the defaults is a file nobody can tell they have changed.

`--hooks` writes a pre-commit hook that runs the tree rules before every commit, and
`--actions` writes a workflow that runs all of them on every pull request. Both are optional
and both are the point: a rule nobody runs is a comment.

```console
$ isu init --prefix APP --hooks --actions
.isu.yml, .git/hooks/pre-commit, .github/workflows/isu.yml
  .isu.yml
  .git/hooks/pre-commit
  .github/workflows/isu.yml
```

Running it twice changes nothing. A file that is already what isu would write is left alone,
and a file that is something else is refused rather than overwritten — pass `--force` when
replacing it is what you meant.

An empty repository has an empty board, and says so rather than printing a blank line:

```console
$ isu board
main · 0 issues · remote refs just now

nothing to show: no issues on any ref isu can see
```

The rules have nothing to complain about yet, which is also worth seeing once:

```console
$ isu check
main · 9 checks · nothing to report · remote refs just now
```

## File something

`isu new` writes the issue folder, generates an id nothing else in the repository is using, and
puts it on a `report/<id>` branch ready for a pull request.

```console
$ isu new --type bug --title "Login retries drop the second attempt" --repro "sign in, fail once, retry within 5s" --owner dana --priority p1
```

It prints the id it allocated, the branch it wrote onto and the commit —
`APP-yynajh · on report/APP-yynajh · commit 7a221455` on the run that built this page, and
something else on yours. The id is a hash of the issue's own fields and eight bytes of
randomness, so allocating one needs no counter, no lock and nobody's agreement. That is the
only thing that works when half the issues in flight are on branches nobody has pushed yet, and
it is why this page cannot print the id and mean it.

The report is on a branch and not on trunk, so the board calls it **awaiting triage** until the
branch merges. That is not a column somebody has to move a card out of; it is a fact about the
repository, and it stops being true when the report lands. A board with all six derived
statuses on it is [on the front page](../index.html#derived).

Nothing is ready to pick up yet, because the only issue there is has not been triaged:

```console
$ isu ready --json=false
nothing is ready: everything open is blocked, claimed or untriaged
```

## Take it, do it, close it

Four commands carry an issue from the queue to trunk. Replace `<id>` with the id `isu new`
printed.

This block is the one place on the site that shows commands without running them, and it is
worth saying why rather than leaving a reader to wonder. `isu claim` writes a random
`Isu-Claim:` nonce, so its commit id differs on every run; the board after it reads
`remote refs just now`, which is a wall clock; and the claimant it prints is whoever `git` is
configured as on the machine that built the page. A page whose bytes have to be identical on
every machine cannot quote any of those three, and inventing them is the one thing this site
does not do. What the loop looks like when it closes is [on the front
page](../index.html#claims), against the sample repository, where the clock and the identities
are fixed: the same command on two claims one merge apart, `in progress` and then `done`.

```sh
isu triage <id> --owner dana --priority p1   # who is answerable, and how urgent
isu ready --json=false                       # the queue, most urgent first
isu claim <id>                               # branch, flip the state, push
# ... write the fix, and the test that proves it ...
isu resolve <id>                             # commit with an Isu-Resolves: trailer
```

`isu claim` creates `isu/<id>`, writes `state: resolved` into the issue, commits with a random
`Isu-Claim:` trailer and pushes — **before any work starts**, so whoever loses the race has
wasted nothing. The push is the compare-and-swap: two claimants write two different commits, so
the second push is not a fast-forward and git refuses it.

Writing `resolved` before the work is done is deliberate. A branch is a proposal; trunk is
where state is true. Until the branch merges, the only thing that reads it is a board that
shows the issue as **in progress**, with the claimant and the age of the claim beside it.
`isu unclaim` puts the state back and leaves the branch standing, because a one-word command
must not throw away work.

## Let the rules argue with you

Claiming an issue and then resolving nothing does not get past `isu check`. A claim writes
`state: resolved` and touches nothing else, which in the file is indistinguishable from a
resolution — so the rule reads the diff instead, and refuses a branch that closed a bug without
changing anything outside `issues/`. There is a worked example of that failure
[on the front page](../index.html#checks).

A failure exits 1 and a warning does not: two people about to do the same work is worth saying
and is not a reason to refuse a pull request.

## Where to go next

- [The data model](data-model.html) — the folder, the frontmatter and the ids.
- [Derived statuses](statuses.html) — every status and the ordered rule that produces it.
- [The check catalogue](checks.html) — the nine rules, and what each one refuses.
- [The JSON contract](json.html) — every field of every payload.
