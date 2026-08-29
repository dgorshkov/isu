package repo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/repo"
)

// Every phase of a read reports the failure that stopped it.
//
// A loader is a sequence of git commands, and each one has its own error
// return. Breaking the first command proves only the first return; the ones
// after it stay claims until something takes them, and they are the returns
// that fire when a repository is large or a disk is full — the least convenient
// moment for a loader to answer with an empty board instead.
//
// So each subcommand is broken in turn, with the rest working, which is also
// the only way to tell the phases apart from outside.

func TestLoadBoardReportsAFailureInEveryPhase(t *testing.T) {
	for _, subcommand := range []string{"for-each-ref", "ls-tree", "diff-tree", "cat-file"} {
		t.Run(subcommand, func(t *testing.T) {
			r := boardFixture(t)

			_, err := failingAt(t, r, subcommand).LoadBoard(
				t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})

			require.ErrorContains(t, err, subcommand,
				"the failure names the command that produced it")
			require.ErrorContains(t, err, "refusing")
		})
	}
}

func TestLoadHistoryReportsAFailureInEveryPhase(t *testing.T) {
	for _, subcommand := range []string{"log", "cat-file"} {
		t.Run(subcommand, func(t *testing.T) {
			r := boardFixture(t)

			_, err := failingAt(t, r, subcommand).LoadHistory(t.Context(), gittest.DefaultBranch)

			require.ErrorContains(t, err, subcommand)
		})
	}
}

func TestLoadRefReportsAFailureInEveryPhase(t *testing.T) {
	for _, subcommand := range []string{"ls-tree", "cat-file"} {
		t.Run(subcommand, func(t *testing.T) {
			r := boardFixture(t)

			_, err := failingAt(t, r, subcommand).LoadRef(t.Context(), gittest.DefaultBranch)

			require.ErrorContains(t, err, subcommand)
		})
	}
}

// The working tree's one git process is the one that asks what is ignored, and
// a loader that could not ask must not answer as though nothing were.
func TestLoadWorktreeReportsAFailureListingIgnoredPaths(t *testing.T) {
	r := boardFixture(t)

	_, err := failingAt(t, r, "ls-files").LoadWorktree(t.Context())

	require.ErrorContains(t, err, "ls-files")
}

// boardFixture is a repository with something on trunk and something on a
// branch, so that every phase of a board read has work to do and a failure in
// any of them is reached.
func boardFixture(t *testing.T) *gittest.Repo {
	t.Helper()

	return gittest.New(t).
		Issue("ISU-7f3akq").Issue("ISU-40b1cc").Commit("two issues").
		Branch("wip").Checkout("wip").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("resolve one").
		Checkout(gittest.DefaultBranch)
}

// failingAt reads the repository through a git that works normally except for
// one subcommand, which exits non-zero.
//
// The shim is the technique gitx already uses to test a git that is not
// installed, pointed at a git that is installed and refuses. Everything else
// passes straight through to the real binary, so the fixture is read exactly as
// it would be until the moment being tested.
func failingAt(t *testing.T, r *gittest.Repo, subcommand string) *repo.Repo {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script; CI is linux and macos")
	}

	shim := filepath.Join(t.TempDir(), "git")

	// The subcommand is the first argument that is neither a global flag nor a
	// global flag's value, and gitx passes two of the latter: `-c
	// core.quotePath=false` and `-C <dir>`. Skipping only what starts with a
	// dash finds the repository path instead and refuses nothing, which is how
	// the first draft of this passed while testing the real git.
	//
	// Everything not refused is handed to the real binary, which the shim finds
	// on PATH — where it still is, because the shim is reached by its absolute
	// path and is not itself on PATH.
	script := fmt.Sprintf(`#!/bin/sh
skip=0
for a in "$@"; do
	if [ "$skip" = 1 ]; then skip=0; continue; fi
	case "$a" in
	-c|-C) skip=1 ;;
	-*) ;;
	%s) echo "fatal: this git is refusing %s" >&2; exit 1 ;;
	*) break ;;
	esac
done
exec git "$@"
`, subcommand, subcommand)

	require.NoError(t, os.WriteFile(shim, []byte(script), 0o700))

	loader, err := repo.Open(r.Dir(), repo.WithGit(gitx.WithBinary(shim)))
	require.NoError(t, err)

	return loader
}
