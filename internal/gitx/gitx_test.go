package gitx_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/gitx"
)

// open is New against a scripted repository, for the tests that only need a
// working handle.
func open(t *testing.T, r *gittest.Repo) *gitx.Git {
	t.Helper()

	g, err := gitx.New(r.Dir())
	require.NoError(t, err)

	return g
}

func TestNewSaysSoWhenGitIsNotInstalled(t *testing.T) {
	r := gittest.New(t)

	_, err := gitx.New(r.Dir(), gitx.WithBinary("isu-no-such-git"))

	require.ErrorIs(t, err, gitx.ErrNotFound)
	require.ErrorContains(t, err, "runtime requirement",
		"a missing git is the one failure every command shares, so it says what to do")
}

func TestNewRefusesADirectoryThatIsNotThere(t *testing.T) {
	_, err := gitx.New(filepath.Join(t.TempDir(), "absent"))

	require.ErrorContains(t, err, "absent")
}

// The plan requires git to be invoked with --no-pager and a clean environment.
// Both are asserted by putting a script named git on the path and reading back
// what it was handed.
func TestGitIsInvokedWithNoPagerAndACleanEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script; CI is linux and macos")
	}

	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	envFile := filepath.Join(dir, "env")
	shim := filepath.Join(dir, "git")

	require.NoError(t, os.WriteFile(shim, []byte(
		"#!/bin/sh\n"+
			"for a in \"$@\"; do printf '%s\\n' \"$a\"; done > \"$ISU_SHIM_ARGS\"\n"+
			"env > \"$ISU_SHIM_ENV\"\n"), 0o700))

	t.Setenv("ISU_SHIM_ARGS", argsFile)
	t.Setenv("ISU_SHIM_ENV", envFile)

	// Every one of these redirects git at a repository other than the one it
	// was pointed at. A process that inherits them — a test run from a hook,
	// from `git bisect run`, from any tool that exports them — must not have
	// them reach git.
	for _, name := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE",
		"GIT_CEILING_DIRECTORIES",
	} {
		t.Setenv(name, "/somewhere/else")
	}

	g, err := gitx.New(t.TempDir(), gitx.WithBinary(shim))
	require.NoError(t, err)

	_, err = g.Output(t.Context(), "rev-parse", "HEAD")
	require.NoError(t, err)

	args := strings.Split(strings.TrimRight(readFile(t, argsFile), "\n"), "\n")
	require.Equal(t, "--no-pager", args[0], "the pager is off before anything else")
	require.Contains(t, args, "rev-parse")

	env := readFile(t, envFile)
	for _, name := range []string{
		"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE=", "GIT_OBJECT_DIRECTORY=",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=", "GIT_COMMON_DIR=", "GIT_NAMESPACE=",
		"GIT_CEILING_DIRECTORIES=",
	} {
		require.NotContains(t, env, name, "%s must not reach git", name)
	}
	require.Contains(t, env, "LC_ALL=C", "output is parsed, so it is parsed in one locale")
	require.Contains(t, env, "GIT_OPTIONAL_LOCKS=0", "reading a repository must not take its lock")
	require.Contains(t, env, "GIT_TERMINAL_PROMPT=0", "a read must never block on a prompt")
}

// A user's own git configuration, credential helpers and hooks are the reason
// PLAN.md shells out rather than using go-git. Scrubbing the environment must
// not take them away.
func TestGitKeepsTheRestOfTheEnvironment(t *testing.T) {
	t.Setenv("ISU_KEEP_ME", "yes")

	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	out, err := open(t, r).Output(t.Context(), "var", "GIT_AUTHOR_IDENT")
	require.NoError(t, err)
	require.Contains(t, out, "@", "git still resolves an identity from the user's configuration")
}

func TestErrorCarriesStderrAndTheArguments(t *testing.T) {
	r := gittest.New(t)

	_, err := open(t, r).Output(t.Context(), "rev-parse", "refs/heads/no-such-branch")
	require.Error(t, err)

	var gerr *gitx.Error
	require.ErrorAs(t, err, &gerr)
	require.NotEmpty(t, gerr.Stderr, "the failure is in stderr, so the error carries it")
	require.Contains(t, gerr.Args, "rev-parse")
	require.NotZero(t, gerr.ExitCode)
	require.Contains(t, err.Error(), "no-such-branch")
}

func TestTimeoutIsReported(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	g, err := gitx.New(r.Dir(), gitx.WithTimeout(time.Nanosecond))
	require.NoError(t, err)

	_, err = g.Output(t.Context(), "rev-parse", "HEAD")
	require.Error(t, err)
	require.True(t,
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"a git that outran its timeout says so rather than looking like a git that failed")
}

func TestProcessesCountsEveryInvocation(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")
	g := open(t, r)

	require.Zero(t, g.Processes())

	_, err := g.RevParse(t.Context(), "HEAD")
	require.NoError(t, err)
	_, err = g.LsTree(t.Context(), "HEAD")
	require.NoError(t, err)

	require.Equal(t, int64(2), g.Processes(),
		"M2-S5 asserts a process count, so something has to be counting")
}

// PLAN.md M2-S1: "Done when `grep -r \"exec.Command\" internal/ | grep -v gitx`
// returns nothing. Add that grep as a test."
func TestNothingOutsideGitxExecutesGit(t *testing.T) {
	// Split so that this file is not its own counterexample.
	needle := "exec." + "Command"

	root := filepath.Join("..") // internal/

	var offenders []string

	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "gitx" {
				return fs.SkipDir
			}

			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		if strings.Contains(readFile(t, path), needle) {
			offenders = append(offenders, path)
		}

		return nil
	}))

	require.Empty(t, offenders,
		"internal/gitx is the only place that may build a git command: "+
			"the user's config, hooks and credential helpers apply because every "+
			"invocation goes through one door")
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path) //nolint:gosec // test paths only
	require.NoError(t, err)

	return string(body)
}
