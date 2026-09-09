package site

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// root is this repository, from the directory these tests run in.
const root = "../.."

// TestEveryCommandInTheContentPlanRuns is M8-S1's own gate.
func TestEveryCommandInTheContentPlanRuns(t *testing.T) {
	t.Parallel()

	dir, err := Fixture(t.Context(), t.TempDir())
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(root, "web", "CONTENT.md"))
	require.NoError(t, err)

	require.NoError(t, Verify(dir, string(body)))
}

// TestTheContentPlanSaysWhatThePageNeeds reads the plan the way the build
// does, so a plan missing a section or a metadata line fails here rather than
// producing a page with a hole in it.
func TestTheContentPlanSaysWhatThePageNeeds(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(root, "web", "CONTENT.md"))
	require.NoError(t, err)

	plan, err := ParsePlan(string(body))
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(plan.Sections), 4,
		"a page that proves fewer than four things is a page nobody believes")

	proofs := 0

	for _, section := range plan.Sections {
		require.NotEmptyf(t, section.ID, "%q has no anchor", section.Title)
		require.NotEmptyf(t, section.Claim, "%q claims nothing", section.Title)
		require.NotEmptyf(t, section.Copy, "%q is a heading with no copy", section.Title)

		if len(section.Samples) > 0 {
			proofs++
		}
	}

	require.GreaterOrEqual(t, proofs, 4, "fewer than four sections show the tool running")

	for _, key := range []string{"title", "tagline", "description", "lede", "install"} {
		value, err := plan.Get(key)
		require.NoError(t, err)
		require.NotEmpty(t, value)
	}
}
