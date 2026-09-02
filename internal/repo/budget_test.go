package repo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The rule behind the two allowances in perf_test.go, asserted rather than
// reasoned about. Both are multiplicative and independent of each other, so the
// case worth naming is the one that takes both — an instrumented binary on a
// slow runner, which is exactly what `make cover` is on macOS in CI.
//
// The allowance is named for the platform it was measured on rather than for
// "not linux": M9-S1 adds a Windows build, and a budget that quietly loosened
// itself on a platform nobody had measured would be a number with no evidence
// behind it.
func TestBudgetAllowsForInstrumentationAndSlowRunners(t *testing.T) {
	const base = time.Second

	for _, tc := range []struct {
		name         string
		instrumented bool
		goos         string
		want         time.Duration
		why          string
	}{
		{
			name: "the runner the budgets were measured on",
			goos: "linux",
			want: base,
			why:  "an uninstrumented binary",
		},
		{
			name:         "instrumented, on that same runner",
			instrumented: true,
			goos:         "linux",
			want:         2 * base,
			why:          "a binary instrumented for coverage",
		},
		{
			name: "a slow runner",
			goos: "darwin",
			want: 2 * base,
			why:  "an uninstrumented binary on darwin",
		},
		{
			name:         "both at once",
			instrumented: true,
			goos:         "darwin",
			want:         4 * base,
			why:          "a binary instrumented for coverage on darwin",
		},
		{
			name: "a platform nobody has measured yet",
			goos: "windows",
			want: base,
			why:  "an uninstrumented binary",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			held, why := budget(base, tc.instrumented, tc.goos)

			require.Equal(t, tc.want, held)
			require.Equal(t, tc.why, why)
		})
	}
}
