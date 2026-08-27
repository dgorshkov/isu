package repo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/repo"
)

// The gate PLAN.md M2-S5 sets. The numbers are the plan's, and they are budgets
// for the slowest runner in CI rather than measurements of anybody's laptop.
const (
	// perfIssues is the repository size §0's own measurements were taken at.
	perfIssues = 5000
	// perfBranches is what `isu board` reads beside trunk. It never reads one
	// ref, so the single-ref number in §0 is not a budget the product's main
	// operation can be held to.
	perfBranches = 200
	// loadRefBudget is the whole of LoadRef on perfIssues issues.
	loadRefBudget = 1500 * time.Millisecond
	// boardBudget is the whole of LoadBoard on perfIssues issues over
	// perfBranches branches.
	boardBudget = 6 * time.Second
)

// TestLoadRefUnder5000IssuesIsFast is one half of the gate: the clock, and the
// process count.
//
// The process count is the assertion that actually prevents the regression. The
// three approaches §0 measured — 13.7 s, 4.9 s and 0.6 s — are the same
// algorithm run a different number of times, so a rewrite that quietly went
// back to a git process per issue would show up here long before it showed up
// as seconds on a machine fast enough to hide it.
func TestLoadRefUnder5000IssuesIsFast(t *testing.T) {
	r := gittest.Generate(t, gittest.Spec{Issues: perfIssues})
	loader := open(t, r)

	started := time.Now()

	set, err := loader.LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	took := time.Since(started)

	require.Equal(t, perfIssues, set.Len())
	require.Empty(t, set.Broken)
	require.Less(t, took, loadRefBudget,
		"LoadRef took %s over %d issues, and the budget is %s", took, perfIssues, loadRefBudget)
	require.Equal(t, int64(2), loader.Processes(),
		"one ls-tree and one cat-file --batch, whatever the repository holds")
}

// The process count does not move with the number of issues. Two sizes, one
// count.
func TestLoadRefSpawnsTwoProcessesWhateverTheIssueCount(t *testing.T) {
	for _, issues := range []int{10, 1000} {
		r := gittest.Generate(t, gittest.Spec{Issues: issues})
		loader := open(t, r)

		set, err := loader.LoadRef(t.Context(), gittest.DefaultBranch)
		require.NoError(t, err)
		require.Equal(t, issues, set.Len())
		require.Equal(t, int64(2), loader.Processes(), "%d issues", issues)
	}
}

// The other half of the gate: the whole board, over refs.
func TestLoadBoardOver200BranchesIsFast(t *testing.T) {
	r := gittest.Generate(t, gittest.Spec{Issues: perfIssues, Branches: perfBranches})
	loader := open(t, r)

	started := time.Now()

	board, err := loader.LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	took := time.Since(started)

	require.Equal(t, perfIssues, board.Trunk.Len())
	require.Len(t, board.Names(), perfBranches)
	require.Less(t, took, boardBudget,
		"LoadBoard took %s over %d issues and %d branches, and the budget is %s",
		took, perfIssues, perfBranches, boardBudget)
}

// Linear in refs, and flat in issues. Three sizes say both things at once:
// holding the branch count and moving the issue count must not move the process
// count at all, and holding the issue count and moving the branch count must
// move it by exactly one per branch.
func TestLoadBoardProcessCountGrowsInRefsAndNotInIssues(t *testing.T) {
	// One for-each-ref, one ls-tree for trunk, one per other ref, one batch.
	const overhead = 3

	for _, tc := range []struct{ issues, branches int }{
		{issues: 20, branches: 5},
		{issues: 400, branches: 5},
		{issues: 400, branches: 40},
	} {
		r := gittest.Generate(t, gittest.Spec{Issues: tc.issues, Branches: tc.branches})
		loader := open(t, r)

		board, err := loader.LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
		require.NoError(t, err)

		require.Len(t, board.Names(), tc.branches)
		require.Equal(t, int64(tc.branches+overhead), loader.Processes(),
			"%d issues over %d branches", tc.issues, tc.branches)
	}
}

func BenchmarkLoadRef(b *testing.B) {
	r := gittest.Generate(b, gittest.Spec{Issues: perfIssues})
	loader, err := repo.Open(r.Dir())
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		set, err := loader.LoadRef(b.Context(), gittest.DefaultBranch)
		if err != nil || set.Len() != perfIssues {
			b.Fatalf("LoadRef: %v", err)
		}
	}
}

func BenchmarkBoard(b *testing.B) {
	r := gittest.Generate(b, gittest.Spec{Issues: perfIssues, Branches: perfBranches})
	loader, err := repo.Open(r.Dir())
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		board, err := loader.LoadBoard(b.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
		if err != nil || len(board.Refs) != perfBranches {
			b.Fatalf("LoadBoard: %v", err)
		}
	}
}

func BenchmarkLoadWorktree(b *testing.B) {
	r := gittest.Generate(b, gittest.Spec{Issues: perfIssues})
	loader, err := repo.Open(r.Dir())
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		set, err := loader.LoadWorktree(b.Context())
		if err != nil || set.Len() != perfIssues {
			b.Fatalf("LoadWorktree: %v", err)
		}
	}
}

func BenchmarkLoadHistory(b *testing.B) {
	r := gittest.Generate(b, gittest.Spec{Issues: perfIssues})
	loader, err := repo.Open(r.Dir())
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		history, err := loader.LoadHistory(b.Context(), gittest.DefaultBranch)
		if err != nil || len(history) != perfIssues {
			b.Fatalf("LoadHistory: %v", err)
		}
	}
}
