package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// What the site build says when git will not answer.
//
// The fixture is a dozen git processes in a row and each of them has an error
// return nobody had ever taken. The lever is the one PLAN.md's definition of
// done names and internal/cli/gitfails_test.go already uses: a git on PATH that
// forwards to the real one and refuses exactly one invocation, chosen by the
// argument it carries.
//
// These tests cannot run in parallel. PATH is process-wide, and so is the point.

// refuseGit puts a git on PATH that fails whenever one of its arguments is the
// given string, and forwards to the real git otherwise.
func refuseGit(t *testing.T, argument string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script; CI is linux and macos")
	}

	binary, err := exec.LookPath("git")
	require.NoError(t, err)

	dir := t.TempDir()

	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = '" + argument + "' ]; then\n" +
		"    echo 'the shim refused this one' >&2\n" +
		"    exit 3\n" +
		"  fi\n" +
		"done\n" +
		"exec '" + binary + "' \"$@\"\n"

	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700)) //nolint:gosec // a test's own script

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestTheFixtureSaysWhichGitWouldNotAnswer(t *testing.T) {
	for _, tt := range []struct{ name, refuse, want string }{
		{"hashing a blob", "hash-object", "building the sample repository"},
		{"writing the commit", "commit-tree", "building the sample repository"},
		{"moving the branch", "update-ref", "building the sample repository"},
		{"pushing to the remote", "push", "building the sample repository"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			refuseGit(t, tt.refuse)

			_, err := Fixture(t.Context(), t.TempDir())
			require.ErrorContains(t, err, tt.want)
			require.ErrorContains(t, err, "the shim refused this one")
		})
	}
}

func TestAWritableRepositorySaysWhichGitWouldNotAnswer(t *testing.T) {
	refuseGit(t, "config")

	_, err := Writable(t.Context(), t.TempDir())
	require.ErrorContains(t, err, "building a repository for the documentation")
}

// The whole build, given a git that will not read a tree. It is the one
// assertion that the failure of a git process reaches the person who ran
// `make site` rather than producing a site with nothing in it.
func TestBuildSaysWhenGitWillNotAnswer(t *testing.T) {
	root := copyWithout(t, "")

	refuseGit(t, "write-tree")

	_, err := Build(t.Context(), root, t.TempDir())
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "the shim refused this one"),
		"the failure names what git said: %v", err)
}
