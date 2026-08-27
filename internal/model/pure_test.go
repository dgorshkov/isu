package model_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// PLAN.md M3-S1: "Pure function over loaded inputs — no git calls inside, and
// no exceptions to that rule later." M3-S4 asks for that as a test in the style
// of the M2-S1 grep, and this is it, in three parts: what this package may
// import, what it may name, and what it costs at runtime.
//
// The rule is not fussiness. A derivation that could run git would run one
// process per issue the first time somebody needed a field the loader had not
// fetched — which is exactly the 13.7 s read path PLAN.md section 0 measured
// and threw away.

// deriveDeps are the packages internal/model may import. internal/repo is on
// the list for its types: a Board, a History and a FirstCommit are what this
// package is handed. What it may not do is hold the thing that loads them,
// which is the next test.
var deriveDeps = map[string]bool{
	"github.com/dgorshkov/isu/internal/config": true,
	"github.com/dgorshkov/isu/internal/issue":  true,
	"github.com/dgorshkov/isu/internal/repo":   true,
}

func TestModelImportsNothingThatCouldRunGit(t *testing.T) {
	for _, path := range sources(t) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		require.NoError(t, err)

		for _, imported := range file.Imports {
			name := strings.Trim(imported.Path.Value, `"`)

			// A standard library path has no dot in its first element, which is
			// what tells it from a module path without a list to maintain.
			if head, _, _ := strings.Cut(name, "/"); !strings.Contains(head, ".") {
				continue
			}

			require.True(t, deriveDeps[name],
				"%s imports %s: derivation reads what internal/repo loaded and "+
					"nothing else, so a new dependency here is a decision, not a diff",
				path, name)
		}
	}
}

func TestModelHoldsNoRepository(t *testing.T) {
	// Split so that this file is not its own counterexample.
	forbidden := []string{"exec." + "Command", "repo." + "Repo"}

	for _, path := range sources(t) {
		body, err := os.ReadFile(path)
		require.NoError(t, err)

		for _, needle := range forbidden {
			require.NotContains(t, string(body), needle,
				"%s names %s: this package is handed what was loaded, and a "+
					"derivation that can reach a repository will eventually reach one",
				path, needle)
		}
	}
}

// The assertion the two above are proxies for. A repository that has been read
// and then derived from spawns nothing more.
func TestDerivingSpawnsNoGitProcesses(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-40b1cc", epicIssue...).
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc")).
		Commit("an epic and its child").
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc"), gittest.State("resolved")).
		Commit("resolve ISU-7f3akq").
		Revert("HEAD").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc"), gittest.State("resolved")).
		Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	loader, err := repo.Open(r.Dir())
	require.NoError(t, err)

	ctx := t.Context()

	loaded, err := loader.LoadBoard(ctx, repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	history, err := loader.LoadHistory(ctx, gittest.DefaultBranch)
	require.NoError(t, err)

	claims, err := loader.LoadFirstCommits(ctx, gittest.DefaultBranch, model.ClaimRefs(loaded))
	require.NoError(t, err)

	spent := loader.Processes()
	require.Positive(t, spent, "the fixture was read, so something ran")

	board := model.Derive(model.Input{
		Loaded: loaded, History: history, Claims: claims,
		Config: defaultConfig(), Now: time.Now(),
	})

	require.Equal(t, spent, loader.Processes())

	// Everything the derivation had to answer, answered without git: a status,
	// an annotation that survived losing to it, a claimant, and a rollup.
	got := item(t, board, "ISU-7f3akq")
	require.Equal(t, model.StatusInProgress, got.Status)
	require.True(t, got.Reopened)
	require.Equal(t, "isu tester", got.Claims[0].Claimant)
	require.Equal(t, model.StatusOpen, statusOf(t, board, "ISU-40b1cc"))
}

// sources lists this package's own Go files, tests excluded: a test may open a
// repository, and several of them have to.
func sources(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var paths []string

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}

		paths = append(paths, name)
	}

	require.NotEmpty(t, paths)

	return paths
}
