//go:build stress

package cli

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file is behind a build tag and out of the default suite, which the plan
// M4-S4 asks for in as many words: repeating a network operation a hundred
// times per CI run buys confidence in the network, not in the code. The
// deterministic rejection test in claim_test.go is what actually guards the
// design; this is what you run when you do not believe it.
//
//	make stress

// claimants is how many clones race for one issue. Exactly one may win.
const claimants = 100

func TestAHundredClaimantsAndExactlyOneWinner(t *testing.T) {
	origin := claimable(t)

	clones := make([]string, claimants)
	for i := range clones {
		clones[i] = clone(t, origin).Dir()
	}

	var (
		mu      sync.Mutex
		winners []string
		losers  int
	)

	var wg sync.WaitGroup

	for _, dir := range clones {
		wg.Add(1)

		go func() {
			defer wg.Done()

			got := isu(t, dir, "--json", "claim", "ISU-openly")

			mu.Lock()
			defer mu.Unlock()

			if got.code == 0 {
				winners = append(winners, dir)

				return
			}

			losers++
		}()
	}

	wg.Wait()

	require.Len(t, winners, 1,
		"the push is the compare-and-swap: two claimants write two different "+
			"commits, so the second push is not a fast-forward")
	require.Equal(t, claimants-1, losers)

	// And every loser left nothing behind.
	for _, dir := range clones {
		if dir == winners[0] {
			continue
		}

		require.NotContains(t, branches(t, dir), "isu/ISU-openly")
	}
}
