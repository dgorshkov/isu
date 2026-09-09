package repo_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/repo"
)

// The dimension M2-S5's gate does not measure.
//
// Every budget in perf_test.go is a claim about how much a repository *holds* —
// five thousand issues, two hundred branches — and both are bounded by what a
// team is working on. The fixture behind them is one commit deep, so nothing
// gates the one dimension bounded by how long the repository has *existed*, and
// which only ever grows: the length of trunk.
//
// LoadHistory is the read that pays for it. `reopened` cannot be answered from
// the current content of any ref, so M2-S4 walks trunk's first-parent chain
// under a pathspec — and git looks at every commit on that chain to apply the
// pathspec, whether or not it touched an issue. Still two processes, but the
// work inside them grows with the project's age rather than its backlog.
//
// Measured on a development machine with 1,000 issues and a fifth of trunk
// touching one — `go test -run '^$' -bench ByTrunkDepth -benchtime 3x
// ./internal/repo/`:
//
//	trunk depth   LoadHistory   LoadBoard
//	          1         76 ms      150 ms
//	      1,000        149 ms         —
//	      5,000        486 ms         —
//	     20,000       1.22 s       120 ms
//
// LoadBoard is flat, which is what it was designed to be: it reads trees at ref
// tips and never walks history, so the difference between its two rows is noise
// rather than growth. LoadHistory is linear in depth, and by twenty thousand
// commits — an ordinary three-year-old repository — it is nine tenths of the
// whole read, on a fixture holding a fifth as many issues as M2-S5's.
//
// Nothing is broken. The budgets in the plan are met, and no product decision
// rests on these numbers today. What they say is where the next one will come
// from: every other cost here is bounded by what a team is working on, and this
// one is bounded by how long they have been working.
//
// **These are benchmarks and not gates, deliberately.** There is no budget in
// the plan to hold trunk depth to, and inventing one from a first measurement is
// how a gate ends up meaning nothing. They are also not cheap — a 20,000-commit
// fixture costs tens of seconds to build — and paying that on every CI run for
// numbers nobody reads is how a pipeline becomes something people learn to
// ignore. `go test ./...` does not run them. What keeps the fixture honest
// between runs is a test in internal/gittest that costs a second.
const depthIssues = 1000

// depthOfTrunk are the trunk lengths worth a measurement. Twenty thousand
// commits is an ordinary three-year-old repository, not a pathological one.
var depthOfTrunk = []int{1, 1000, 5000, 20000}

func BenchmarkLoadHistoryByTrunkDepth(b *testing.B) {
	for _, commits := range depthOfTrunk {
		b.Run(fmt.Sprintf("%d-commits", commits), func(b *testing.B) {
			loader := deepFixture(b, commits, 0)

			b.ResetTimer()

			for b.Loop() {
				history, err := loader.LoadHistory(b.Context(), gittest.DefaultBranch)
				if err != nil || len(history) != depthIssues {
					b.Fatalf("LoadHistory: %v", err)
				}
			}
		})
	}
}

// The contrast that makes the point: reading the refs costs the same at any
// depth, because it reads trees at tips and never walks.
func BenchmarkLoadBoardByTrunkDepth(b *testing.B) {
	const branches = 20

	for _, commits := range []int{1, 20000} {
		b.Run(fmt.Sprintf("%d-commits", commits), func(b *testing.B) {
			loader := deepFixture(b, commits, branches)

			b.ResetTimer()

			for b.Loop() {
				board, err := loader.LoadBoard(
					b.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
				if err != nil || board.Trunk.Len() != depthIssues {
					b.Fatalf("LoadBoard: %v", err)
				}
			}
		})
	}
}

// deepFixture builds a repository whose trunk is the given number of commits
// long, a fifth of them changing an issue — trunk in a repository using isu is
// mostly code, and a commit that touches nothing under issues/ still has to be
// walked.
func deepFixture(b *testing.B, commits, branches int) *repo.Repo {
	b.Helper()

	r := gittest.Generate(b, gittest.Spec{
		Issues: depthIssues, Branches: branches,
		Commits: commits, TouchesIssues: commits / 5,
	})

	loader, err := repo.Open(r.Dir())
	require.NoError(b, err)

	return loader
}
