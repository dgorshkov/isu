package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// What isu says when git will not answer.
//
// Every command here is a few git processes in a row, and each of them has an
// error return that until now nobody had ever taken. An error return nobody has
// ever taken is a message nobody has ever read, and the plan's coverage floor
// exists to say so.
//
// The lever is a git on PATH that forwards to the real one and refuses exactly
// one invocation — the one whose arguments contain a given string. That is
// precise enough to fail `git log --first-parent` without failing the
// `git log --reverse` beside it, and honest: it is the same shim technique
// M2-S1's own tests use, pointed at the whole product instead of one wrapper.
//
// These tests cannot run in parallel. PATH is process-wide, and so is the point.

// refuseGit puts a git on PATH that fails whenever one of its arguments is the
// given string, and forwards to the real git otherwise.
func refuseGit(t *testing.T, argument string) {
	t.Helper()

	refuseGitAfter(t, argument, 0)
}

// refuseGitAfter is refuseGit that lets the first skip matching invocations
// through.
//
// A command asks git the same question more than once — which branch am I on,
// what does this ref resolve to — and the error return after the second is not
// the error return after the first. Counting is how a test reaches the second
// one without inventing a seam in the code to reach it through.
func refuseGitAfter(t *testing.T, argument string, skip int) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script; CI is linux and macos")
	}

	binary, err := exec.LookPath("git")
	require.NoError(t, err)

	dir := t.TempDir()
	counter := filepath.Join(dir, "seen")

	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = " + shellQuote(argument) + " ]; then\n" +
		"    seen=$(cat " + shellQuote(counter) + " 2>/dev/null || echo 0)\n" +
		"    echo $((seen + 1)) > " + shellQuote(counter) + "\n" +
		"    if [ \"$seen\" -ge " + shellQuote(itoa(skip)) + " ]; then\n" +
		"      echo 'the shim refused this one' >&2\n" +
		"      exit 3\n" +
		"    fi\n" +
		"  fi\n" +
		"done\n" +
		"exec " + shellQuote(binary) + " \"$@\"\n"

	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700)) //nolint:gosec // a test's own script

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func TestWhatEachCommandSaysWhenGitRefusesOneThing(t *testing.T) {
	// The argument each row refuses is chosen to hit one call and not its
	// neighbours: --first-parent is the trunk history walk, --reverse is the
	// claim lookup beside it, and refs/remotes/ is the freshness read where
	// refs/heads/ is the board.
	tests := []struct {
		name    string
		refuse  string
		fixture func(*testing.T) *gittest.Repo
		args    []string
		// skip lets the first invocations through, for a command that asks git
		// the same question twice and whose second answer is the one under
		// test.
		skip int
	}{
		{
			name: "the trunk history walk", refuse: "--first-parent",
			fixture: board, args: []string{"board"},
		},
		{
			// The claim lookup is one `git log` per claiming branch, and its
			// range names the branch — which the history walk beside it does
			// not, so refusing this reaches one and not the other.
			name: "the claim lookup", refuse: gittest.DefaultBranch + "..refs/heads/isu/ISU-inprog",
			fixture: board, args: []string{"board"},
		},
		{
			name: "the freshness read", refuse: "refs/remotes/",
			fixture: board, args: []string{"board"},
		},
		{
			// A repository whose trunk is called neither main nor master falls
			// back to HEAD, which is the only case that has to ask.
			name: "asking which branch we are on", refuse: "symbolic-ref",
			fixture: unconventionalTrunk, args: []string{"board"},
		},
		{
			name: "asking which branch a write would land on", refuse: "symbolic-ref",
			fixture: onABranchWithAnIssue, args: []string{"resolve", "ISU-openly"},
		},
		{
			name: "asking who the user is", refuse: "user.name",
			fixture: configured,
			args:    []string{"new", "--no-branch", "--title", "x", "--repro", "y"},
		},
		{
			name: "asking who is commenting", refuse: "user.name",
			fixture: commentable, args: []string{"comment", "ISU-openly", "-m", "hello"},
		},
		{
			name:    "resolving the claim branch before writing to it",
			refuse:  "refs/heads/isu/ISU-openly",
			fixture: claimed, args: []string{"unclaim", "ISU-openly"},
		},
		{
			name: "listing the remotes", refuse: "remote",
			fixture: claimable, args: []string{"claim", "ISU-openly"},
		},
		{
			name: "fetching", refuse: "fetch",
			fixture: func(t *testing.T) *gittest.Repo { return board(t).WithRemote() },
			args:    []string{"--fetch", "board"},
		},
		{
			name: "resolving a revision", refuse: "rev-parse",
			fixture: claimable, args: []string{"claim", "ISU-openly"},
		},
		{
			name: "hashing the blob it is about to commit", refuse: "hash-object",
			fixture: claimable, args: []string{"claim", "ISU-openly"},
		},
		{
			name: "building the index it commits from", refuse: "update-index",
			fixture: claimable, args: []string{"claim", "ISU-openly"},
		},
		{
			name: "writing the tree", refuse: "write-tree",
			fixture: claimable, args: []string{"claim", "ISU-openly"},
		},
		{
			name: "writing the commit", refuse: "commit-tree",
			fixture: claimable, args: []string{"claim", "ISU-openly"},
		},
		{
			name: "moving the ref", refuse: "update-ref",
			fixture: claimable, args: []string{"claim", "ISU-openly"},
		},
		{
			name: "staging what it wrote", refuse: "add",
			fixture: onABranchWithAnIssue, args: []string{"resolve", "ISU-openly"},
		},
		{
			name: "switching to the report branch", refuse: "switch",
			fixture: configured,
			args:    []string{"new", "--title", "x", "--repro", "y"},
		},
		{
			name: "listing an issue's folder", refuse: "issues/ISU-onbrch/",
			fixture: onABranchOnly, args: []string{"show", "ISU-onbrch"},
		},
		{
			name: "listing trunk for the checks", refuse: "ls-tree",
			fixture: board, args: []string{"check"},
		},
		{
			// The ref under review is resolved before anything is loaded, so
			// refusing its name reaches that lookup and not the board's.
			name: "resolving the ref under review", refuse: "refs/heads/isu/ISU-openly",
			fixture: onABranchWithAnIssue, args: []string{"check"},
		},
		{
			// The trunk name is worked out before anything is loaded, and it
			// asks git what origin calls its default branch; the ref under
			// review is the next thing to ask.
			name: "asking which branch the checks are about", refuse: "symbolic-ref",
			fixture: board, args: []string{"check"}, skip: 1,
		},
		{
			// Three walks name --no-renames in a run over this fixture: the
			// board's diff of its one other branch, the trunk history, and the
			// branch's own commits. The third is this one.
			name: "walking the branch's own commits", refuse: "--no-renames",
			fixture: aheadOfTrunkAlone, args: []string{"check"}, skip: 2,
		},
		{
			// And two batches precede the branch's: the board's, and the one
			// the trunk history feeds with the blobs it named.
			name: "reading the blobs the branch's commits name", refuse: "cat-file",
			fixture: aheadOfTrunkAlone, args: []string{"check"}, skip: 2,
		},
		{
			name: "weighing what lives beside every issue", refuse: "-l",
			fixture: board, args: []string{"check"},
		},
		{
			name: "finding where the branch left trunk", refuse: "merge-base",
			fixture: aheadOfTrunk, args: []string{"check"},
		},
		{
			name: "diffing the branch against trunk", refuse: "--name-only",
			fixture: aheadOfTrunk, args: []string{"check"},
		},
		{
			name: "asking what git would not track", refuse: "ls-files",
			fixture: board, args: []string{"check", "--worktree"},
		},
		{
			// The working-tree run asks git twice what it would not track:
			// once for the issues and once for the files beside them.
			name: "asking it again for the files beside them", refuse: "ls-files",
			fixture: board, args: []string{"check", "--worktree"}, skip: 1,
		},
		{
			// The importer's evidence scan is the only walk in an import, so
			// refusing this reaches it and nothing else.
			name:   "walking history for the commits that resolved these issues",
			refuse: "--first-parent", fixture: importable,
			args: []string{"import", "github", "--dump", dumpPath},
		},
		{
			name: "asking who owns this repository's hooks", refuse: "core.hooksPath",
			fixture: configured, args: []string{"init", "--hooks"},
		},
		{
			name: "finding the directory the hooks live in", refuse: "--absolute-git-dir",
			fixture: configured, args: []string{"init", "--hooks"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The fixture is built with the real git, and only then is the
			// shim put in front of it.
			r := tt.fixture(t)
			refuseGitAfter(t, tt.refuse, tt.skip)

			got := isu(t, r.Dir(), tt.args...)

			require.Equalf(t, 1, got.code, "stdout was: %s", got.stdout)
			require.NotEmpty(t, got.stderr, "a command that could not run says why")
		})
	}
}

// unconventionalTrunk is a repository whose trunk is called something isu does
// not guess, so that every command has to ask git which branch HEAD is on.
func unconventionalTrunk(t *testing.T) *gittest.Repo {
	t.Helper()

	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("On an unusually named trunk")).
		Commit("report ISU-openly")

	r.Git("branch", "-m", gittest.DefaultBranch, "development")

	return r
}

// onABranchWithAnIssue is onBranch with no options, which the tables here need
// as a plain function of a *testing.T.
func onABranchWithAnIssue(t *testing.T) *gittest.Repo {
	t.Helper()

	return onBranch(t)
}

// aheadOfTrunk is a repository checked out on a branch that has a commit trunk
// does not, which is the only shape the branch rules have anything to load for.
func aheadOfTrunk(t *testing.T) *gittest.Repo {
	t.Helper()

	return board(t).
		Branch("isu/ISU-openly").Checkout("isu/ISU-openly").
		File("login.go", "package login\n").Commit("fix the retry")
}

// aheadOfTrunkAlone is aheadOfTrunk with exactly one branch beside trunk and
// exactly one issue, so that the git processes a run spends can be counted.
func aheadOfTrunkAlone(t *testing.T) *gittest.Repo {
	t.Helper()

	return configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Something to resolve")).
		Commit("report ISU-openly").
		Branch("isu/ISU-openly").Checkout("isu/ISU-openly").
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Something to resolve"), gittest.State("resolved")).
		File("login.go", "package login\n").
		Commit("claim and fix ISU-openly")
}

// onABranchOnly is a repository whose only issue is a report on a branch, so
// that reading its folder has to go through git.
func onABranchOnly(t *testing.T) *gittest.Repo {
	t.Helper()

	return configured(t).
		Branch("report/ISU-onbrch").Checkout("report/ISU-onbrch").
		Issue("ISU-onbrch", gittest.Owner("support"), gittest.Type("chore"),
			gittest.Title("Filed from a branch"),
			gittest.Comment("2026-08-25-support-01", "Still happening.\n"),
			gittest.Comment("2026-08-26-support-01", "And again today.\n")).
		Commit("report ISU-onbrch").
		Checkout(gittest.DefaultBranch)
}

func TestGitRefusingRevParseAfterACommitIsStillReported(t *testing.T) {
	// `git commit` writes the commit and a second process reads back what it
	// wrote, so there is a failure that arrives after the work is done.
	r := onABranchWithAnIssue(t)
	refuseGit(t, "rev-parse")

	got := isu(t, r.Dir(), "resolve", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
}

func TestGitRefusingConfigIsNotTheSameAsAnUnsetKey(t *testing.T) {
	// `git config --get` exits 1 and says nothing when a key is unset, which is
	// an answer. Anything else is a problem, and the two must not be confused:
	// a key nobody can read is not a key nobody set.
	r := configured(t)
	s := openSession(t, r.Dir())

	unset, err := s.git.Config(t.Context(), "isu.nothing.set.here")
	require.NoError(t, err)
	require.Empty(t, unset)

	// A key with no section is a malformed request, which git reports rather
	// than answering.
	_, err = s.git.Config(t.Context(), "nosection")
	require.Error(t, err)
}

func TestGitRefusingTheSecondTimeItIsAskedTheSameThing(t *testing.T) {
	// The error returns that come after a command has already asked git the
	// same question once and been answered.
	tests := []struct {
		name    string
		refuse  string
		skip    int
		fixture func(*testing.T) *gittest.Repo
		args    []string
	}{
		{
			// resolve asks which branch it is on to refuse trunk, and asks
			// again afterwards to say where the commit went.
			name: "which branch, after the refusal check", refuse: "symbolic-ref", skip: 1,
			fixture: onABranchWithAnIssue, args: []string{"resolve", "ISU-openly"},
		},
		{
			name: "which branch, after drop's refusal check", refuse: "symbolic-ref", skip: 1,
			fixture: onABranchWithAnIssue,
			args:    []string{"drop", "ISU-openly", "--reason", "no", "--resolution", "wontfix"},
		},
		{
			// unclaim looks the claiming branch up to check it is there, and
			// the commit it builds looks it up again to hang off it.
			name:    "resolving the claim branch, after the existence check",
			refuse:  "refs/heads/isu/ISU-openly",
			skip:    1,
			fixture: claimed, args: []string{"unclaim", "ISU-openly"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.fixture(t)
			refuseGitAfter(t, tt.refuse, tt.skip)

			got := isu(t, r.Dir(), tt.args...)

			require.Equalf(t, 1, got.code, "stdout was: %s", got.stdout)
			require.NotEmpty(t, got.stderr)
		})
	}
}

func TestGitRefusingTheClaimBranchLookup(t *testing.T) {
	// claim asks whether the branch is already here before it does anything
	// else, and a git that will not answer is not the same as an answer of no.
	r := claimable(t)
	refuseGit(t, "refs/heads/isu/ISU-openly")

	got := isu(t, r.Dir(), "claim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
}

func TestGitRefusingTheTriageBranchLookup(t *testing.T) {
	r := triageable(t, "prefix: ISU\n")
	refuseGit(t, "refs/heads/triage/ISU-triage")

	got := isu(t, r.Dir(), "triage", "ISU-triage", "--owner", "dmitry")

	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
}

func TestGitRefusingTrunkWhereAWriteWouldLand(t *testing.T) {
	// --ref names trunk outright, so nothing else resolves that exact string
	// and the refusal lands in the check that keeps a resolve off trunk.
	r := onABranchWithAnIssue(t)
	refuseGit(t, "refs/heads/"+gittest.DefaultBranch)

	got := isu(t, r.Dir(), "--ref", "refs/heads/"+gittest.DefaultBranch,
		"resolve", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
}

func TestGitRefusingTheRemotesOnAFetchAndOnADirectTriage(t *testing.T) {
	for _, tt := range []struct {
		name    string
		fixture func(*testing.T) *gittest.Repo
		args    []string
	}{
		{
			name:    "before a fetch",
			fixture: func(t *testing.T) *gittest.Repo { return board(t).WithRemote() },
			args:    []string{"--fetch", "board"},
		},
		{
			name: "after a direct triage",
			fixture: func(t *testing.T) *gittest.Repo {
				return triageable(t, "prefix: ISU\ndirect_triage: true\n").WithRemote()
			},
			args: []string{"triage", "ISU-triage", "--owner", "dmitry", "--push"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.fixture(t)
			refuseGit(t, "remote")

			got := isu(t, r.Dir(), tt.args...)

			require.Equal(t, 1, got.code)
			require.NotEmpty(t, got.stderr)
		})
	}
}

func TestGitRefusingSymbolicRefUnderDirectTriage(t *testing.T) {
	// --push has to know which branch trunk is before it can write to it.
	r := triageable(t, "prefix: ISU\ndirect_triage: true\n")
	r.Git("branch", "-m", gittest.DefaultBranch, "development")

	// Three ask before the one that decides which branch --push writes to.
	refuseGitAfter(t, "symbolic-ref", 2)

	got := isu(t, r.Dir(), "triage", "ISU-triage", "--owner", "dmitry", "--push")

	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
}

func TestClaimHolderAnswersNothingWhenTheLookupFails(t *testing.T) {
	// Losing a race and being unable to name who won are different failures,
	// and the second must not turn into a wrong name.
	r := claimed(t)
	s := openSession(t, r.Dir())

	refuseGit(t, "--reverse")

	require.Empty(t, s.claimHolder(t.Context(), "isu/ISU-openly"))
}

// claimed is a repository with one issue claimed on isu/<id>.
func claimed(t *testing.T) *gittest.Repo {
	t.Helper()

	r := claimable(t)
	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)

	return r
}
