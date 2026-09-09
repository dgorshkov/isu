# First contact with real repositories

Everything before this ran against fixtures written by the same person who wrote
the assumptions, so this is the first place the world gets a vote. M4-S8 put it
deliberately before the TUI and the importers were built on top of those
assumptions.

Read paths only. Nothing was written to any of these repositories, nothing was
pushed, and nothing was imported.

## What was run, and against what

Three full clones, chosen for three different shapes rather than three different
logos. Measured on the CI-class machine this milestone was built on, best of
three runs, with a warm page cache.

| | openssl/openssl | facebook/react | golang/go |
|---|---|---|---|
| chosen for | submodules | heavy squash-merge, many refs | the biggest tree |
| clone size | 515 MB | 1.1 GB | 663 MB |
| commits on trunk | 40,883 | 21,659 | 67,438 |
| first commit | 1998-12-21 | 2013-05-29 | 1972-07-18 (backdated) |
| files at trunk | 6,213 | 7,205 | 15,865 |
| submodules | 11 | 0 | 0 |
| local branches | 1 | 1 | 1 |
| remote-tracking refs | 45 | 967 | 66 |
| tags | 443 | 174 | 494 |

None of them has an `issues/` directory, so what these numbers measure is how
isu's read paths behave against real repository *shapes* — depth of history,
size of tree, number of refs, gitlinks in the tree. What loading five thousand
issue files costs is measured by M2-S5's gate, on repositories built for it.

| command | openssl | react | golang/go |
|---|---|---|---|
| `isu board` | 606 ms | 277 ms | 949 ms |
| `isu board --json` | 608 ms | 278 ms | 924 ms |
| `isu ready` | 618 ms | 278 ms | 952 ms |
| `isu show <unknown id>` | 614 ms | 283 ms | 940 ms |

All three load. Nothing crashed, nothing hung, and no output was wrong.

## Where the time goes, and what that means at scale

Every one of those numbers is almost entirely one git process:

| stage | openssl | react | golang/go |
|---|---|---|---|
| `for-each-ref refs/heads/` | 5 ms | 4 ms | 4 ms |
| `ls-tree -r <trunk> -- issues/` | 4 ms | 4 ms | 3 ms |
| **`log --first-parent --raw -- issues`** | **666 ms** | **323 ms** | **1050 ms** |
| `ls-tree -r <trunk>` (whole tree, for scale) | 11 ms | 11 ms | 23 ms |

The trunk history walk is the whole cost, it scales with the number of commits
in the repository and not with the number of issues in it — about **15 µs per
commit**, flat across all three — and **it is paid by every command**, including
`isu show` of one issue that does not exist.

That is M2-S4 working as designed: `reopened` is the one status that cannot be
answered from the current content of any ref, so the history is walked once in
the loader rather than once per issue. What the field adds is the constant. At
a million commits — Chromium is past that — this is fifteen seconds on every
command, and no amount of issue-side optimisation touches it.

**Written down rather than fixed**, because fixing it is a design question this
story is not the place to answer. The shape of an answer: nothing needs
`reopened` except a board and a show, and both could ask for it lazily; or the
walk could stop at the oldest issue's `created` date, which is a bound the file
already carries.

## The one defect, fixed

A `--ref` that does not resolve answered with:

```
isu: git ls-tree -r -z --full-tree refs/heads/nope -- issues: unknown revision:
exit status 128: fatal: Not a valid object name refs/heads/nope
```

which tells somebody who typed one flag about four they have never heard of. It
now leads with `cannot read refs/heads/nope as trunk` and keeps git's sentence
after it, where it helps rather than confuses.
`TestARefThatDoesNotResolveNamesTheRefAndNotThePlumbing` is the regression test.

## Known limitations, written down

### The board cannot see anybody else's claims

**This is the big one.** `isu board` reads trunk and every ref matching
`refs/heads/` — every *local* branch. A claim is a branch pushed to the remote,
so alice's claim reaches bob as `refs/remotes/origin/isu/<ID>`, which the board
does not read. Bob's board shows the issue as `open` and lets him claim it.

Contention across a team — which is the thing claiming exists to prevent — is
therefore invisible. So is every report on somebody else's `report/*` branch.
In these three clones the board sees **one** ref where the remote has 45, 967
and 66.

The freshness line is the evidence that this is not what was intended: it exists
because "two engineers looking at the same repo with different fetch ages see
different contention" (M4-S2), and fetch age cannot affect contention at all
unless remote-tracking refs feed it.

Reading `refs/remotes/` as well is one line in `repo.DefaultRefPattern`, and
what it costs is measurable: 967 `diff-tree` calls against react's trunk took
**3.4 s**, or 3.5 ms a ref. That is within M2-S5's six-second board budget for
react and would blow it for a repository with three thousand branches.

It is left alone here because it is not M4's to change: the ref pattern is
M2's, and de-duplicating a claim that is both a local branch and its own
remote-tracking ref — otherwise every claimant's own board reads their claim as
contended with themselves — is M3's contention rule. It wants a story of its
own, and it should get one before M5-S5 reports contention to anybody.

### fetch_warn_hours measures the repository, not the fetch

Freshness is the age of the newest remote-tracking ref, which is what M4-S2 asks
for in as many words. In a repository nobody has pushed to for a week it warns
that the fetch is stale one second after fetching: react was cloned minutes
before the run and read `remote refs 3d ago — older than 24h, run with --fetch`,
because react's newest branch had not moved in three days.

The words and the intent diverge on a quiet repository. What would measure fetch
age is the mtime of `.git/FETCH_HEAD`, which is a fact about this clone rather
than about the project. Recorded rather than changed, because the current
behaviour is what M4-S2 specified and the amendment belongs in the
[design record](design.html) first.

### Submodules are a non-event, and the interesting case does not exist

openssl carries 11 submodules; one was initialised and the board read the
repository in 665 ms, unchanged. That is because `issues/` and the submodule
paths do not intersect, and nothing in isu walks the working tree outside
`issues/`.

The case that would matter — a submodule mounted *at* `issues/` — appears in
none of these repositories and probably in none at all. `ls-tree -r` reports a
gitlink as type `commit`, which the loader would list and find no `README.md`
under, so a repository shaped that way would read as having no issues. Untested
against anything real, so: a known unknown rather than a known limitation.

### isu cannot read a repository until something writes a file into it

Every command against all three repositories failed identically until
`.isu.yml` existed:

```
isu: <repo> has no .isu.yml: isu needs one line of configuration to know what
to call an issue, and `isu init` writes it
```

This is the open question the [data model](data-model.html) raised and asked to be settled before
M4-S1, and it is why `isu init` exists. The field notes confirm the answer was
worth making: the first thing anybody does with isu in an existing repository
is hit this, and a tool whose first interaction is an error about a file the
user has never heard of is a tool they close.

### A tag or an old commit as trunk is much cheaper, for the obvious reason

`--ref OpenSSL_1_0_1` read openssl's board in 33 ms against 606 ms for trunk,
and `--ref v15.0.0` read react's in 91 ms against 277 ms. The history walk is
the cost and a tag from 2012 has less of it behind it. Worth knowing when
somebody reports that isu is fast on their machine and slow on yours: the
question is how much history is behind the ref, not how many issues are in it.

## What was not tested

- A repository with tens of thousands of issue folders in a real project's
  history. Nobody has one yet; M2-S5's generated fixture is the closest thing
  and it is a fixture.
- Windows. CI is linux and macos, and so was this.
- A repository behind a credential helper that prompts. `--fetch` was run
  against public HTTPS remotes only.
