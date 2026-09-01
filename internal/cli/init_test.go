package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// A hook and a pipeline nobody can run twice are a hook and a pipeline nobody
// puts in a setup script. So the assertion that matters most here is the one
// about the second run: it changes nothing, and it says so.

func TestInitWritesTheHookAndThePipeline(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).File("README.md", "a repository\n").Commit("first")

	got := isu(t, r.Dir(), "init", "--prefix", "NEW", "--hooks", "--actions").ok(t)

	require.Contains(t, got.stdout, ".isu.yml")
	require.Contains(t, got.stdout, "pre-commit")
	require.Contains(t, got.stdout, ".github/workflows/isu.yml")

	hook := filepath.Join(r.Dir(), ".git", "hooks", "pre-commit")

	body, err := os.ReadFile(hook) //nolint:gosec // a path this test just wrote
	require.NoError(t, err)
	require.Contains(t, string(body), "isu check --scope tree",
		"the branch rules cannot be asked before the commit exists")

	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(hook)
		require.NoError(t, statErr)
		require.NotZero(t, info.Mode()&0o111,
			"a hook that is not executable is a hook git ignores without saying so")
	}

	workflow, err := os.ReadFile(filepath.Join(r.Dir(), ".github", "workflows", "isu.yml"))
	require.NoError(t, err)
	require.Contains(t, string(workflow), "isu check",
		"the pipeline is where the branch rules run")
	require.Contains(t, string(workflow), "fetch-depth: 0",
		"a shallow clone has no merge base to measure a branch from")
}

func TestInitTwiceChangesNothing(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).File("README.md", "a repository\n").Commit("first")

	isu(t, r.Dir(), "init", "--prefix", "NEW", "--hooks", "--actions").ok(t)
	r.Git("add", ".")
	r.Commit("set isu up")

	again := isu(t, r.Dir(), "init", "--prefix", "NEW", "--hooks", "--actions").ok(t)

	require.Contains(t, again.stdout, "nothing to do")
	require.Empty(t, r.Git("status", "--porcelain"),
		"a second run is a zero-length diff, which is what makes this safe to "+
			"put in a setup script")

	written := decode[Write](t,
		isu(t, r.Dir(), "--json", "init", "--prefix", "NEW", "--hooks").ok(t))
	require.Empty(t, written.Paths, "an empty list is a list")
}

func TestInitRefusesToOverwriteSomebodyElsesFile(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, path, body string
		args             []string
	}{
		{
			name: "a hook",
			path: filepath.Join(".git", "hooks", "pre-commit"),
			body: "#!/bin/sh\nmake lint\n",
			args: []string{"init", "--prefix", "NEW", "--hooks"},
		},
		{
			name: "a workflow",
			path: filepath.Join(".github", "workflows", "isu.yml"),
			body: "name: something else\n",
			args: []string{"init", "--prefix", "NEW", "--actions"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := gittest.New(t).File("README.md", "a repository\n").Commit("first")

			path := filepath.Join(r.Dir(), tt.path)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o644))

			got := isu(t, r.Dir(), tt.args...)

			require.Equal(t, 1, got.code)
			require.Contains(t, got.stderr, "not what isu writes")
			require.Contains(t, got.stderr, "--force")

			kept, err := os.ReadFile(path) //nolint:gosec // a path this test just wrote
			require.NoError(t, err)
			require.Equal(t, tt.body, string(kept))

			isu(t, r.Dir(), append(tt.args, "--force")...).ok(t)

			replaced, err := os.ReadFile(path) //nolint:gosec // a path this test just wrote
			require.NoError(t, err)
			require.NotEqual(t, tt.body, string(replaced))
		})
	}
}

// The hook goes where git looks for hooks, which is not always .git/hooks. A
// hook written into the wrong directory is a hook that silently never runs.
func TestTheHookGoesWhereGitLooksForIt(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).File("README.md", "a repository\n").Commit("first")
	r.Git("config", "core.hooksPath", "githooks")

	isu(t, r.Dir(), "init", "--prefix", "NEW", "--hooks").ok(t)

	body, err := os.ReadFile(filepath.Join(r.Dir(), "githooks", "pre-commit"))
	require.NoError(t, err)
	require.Contains(t, string(body), "isu check")

	require.NoFileExists(t, filepath.Join(r.Dir(), ".git", "hooks", "pre-commit"))
}

// The hook a repository already has is the whole point of running init in one:
// a repository that adopts isu after it has a pipeline should get the parts it
// is missing and keep the parts it has.
func TestInitAddsTheFilesAConfiguredRepositoryIsMissing(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "init", "--hooks").ok(t)

	require.Contains(t, got.stdout, "pre-commit")
	require.NotContains(t, got.stdout, ".isu.yml",
		"the configuration was already there and is not this run's to touch")
}

func TestInitStillNeedsAPrefixWhenThereIsNoConfiguration(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).File("README.md", "a repository\n").Commit("first")

	got := isu(t, r.Dir(), "init", "--hooks")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "needs --prefix")
	require.NoFileExists(t, filepath.Join(r.Dir(), ".git", "hooks", "pre-commit"),
		"a hook that runs isu check in a repository isu cannot read is a hook "+
			"that fails every commit")
}

// The hook is a shell script, and the two things that matter about it are
// whether git runs it at all and whether its exit status stops the commit.
// What `isu check` itself decides is asserted directly, in the rule tests.

// stub writes an executable script, for the isu a hook finds on PATH.
func stub(t *testing.T, dir, name, body string) {
	t.Helper()

	path := filepath.Join(dir, name)

	require.NoError(t, os.WriteFile(path, []byte(body), 0o755)) //nolint:gosec // it is a script
}

func TestGitRunsTheHookAndItsFailureStopsTheCommit(t *testing.T) {
	// Not parallel: PATH is process-wide, and which isu the hook finds on it is
	// the whole subject.
	r := configured(t)

	isu(t, r.Dir(), "init", "--hooks").ok(t)

	bin := t.TempDir()
	stub(t, bin, "isu", "#!/bin/sh\necho 'isu: the repository is broken' >&2\nexit 1\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	r.File("login.go", "package login\n")

	_, err := r.Try("commit", "-m", "work")
	require.Error(t, err, "a hook that fails stops the commit")
	require.Contains(t, err.Error(), "the repository is broken")
}

func TestTheHookLetsTheCommitThroughWhenIsuIsNotOnPath(t *testing.T) {
	// Not parallel: PATH again. It is set to the directory git itself is in,
	// which is the closest a test gets to a machine where isu was never
	// installed — and a hook that stood between somebody and their commit for
	// that would be a hook the whole team deletes on its first day.
	r := configured(t)

	isu(t, r.Dir(), "init", "--hooks").ok(t)

	git, err := exec.LookPath("git")
	require.NoError(t, err)

	t.Setenv("PATH", filepath.Dir(git))

	r.File("login.go", "package login\n")

	_, err = r.Try("commit", "-m", "work")
	require.NoError(t, err)
}
