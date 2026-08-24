package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pipelines are the CI definitions this repository ships, one per forge. Both
// must exist: a repository that is green on one forge and unbuilt on the other
// has no second opinion, which is the entire reason there are two.
var pipelines = []string{
	filepath.Join("..", ".github", "workflows", "ci.yml"),
	filepath.Join("..", ".gitlab-ci.yml"),
}

var (
	// A rule is a line starting in column zero with a target name and a colon
	// that is not an assignment. Dotted names — .PHONY and friends — are
	// directives rather than targets and are skipped.
	makeRuleRE = regexp.MustCompile(`(?m)^([A-Za-z0-9_][A-Za-z0-9_.\-/]*)[ \t]*:[^=]?`)

	// `make` as a whole word, followed by the rest of its command line.
	makeCallRE = regexp.MustCompile(`(?:^|[\s"'|&;(])make[ \t]+([^\n|&;)"']*)`)
)

// makeTargets returns every target defined by the Makefile at the repository
// root.
func makeTargets(t *testing.T) map[string]bool {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("..", "Makefile"))
	require.NoError(t, err, "the Makefile is what both pipelines call")

	targets := map[string]bool{}
	for _, m := range makeRuleRE.FindAllStringSubmatch(string(body), -1) {
		targets[m[1]] = true
	}
	require.NotEmpty(t, targets, "no rules parsed out of the Makefile")

	return targets
}

// makeInvocations returns the targets a pipeline file asks make to run, in the
// order they appear.
func makeInvocations(t *testing.T, path string) []string {
	t.Helper()

	body, err := os.ReadFile(path)
	require.NoError(t, err, "%s must exist", path)

	var called []string
	for _, m := range makeCallRE.FindAllStringSubmatch(string(body), -1) {
		for _, word := range strings.Fields(m[1]) {
			// Flags and variable overrides are make's business, not targets.
			if strings.HasPrefix(word, "-") || strings.Contains(word, "=") {
				continue
			}
			called = append(called, word)
		}
	}

	return called
}

// TestMakefileTargetsExist is what keeps the two forges honest: neither
// pipeline may grow a step the other cannot run, because the only thing either
// of them is allowed to say is the name of a make target.
func TestMakefileTargetsExist(t *testing.T) {
	targets := makeTargets(t)

	for _, pipeline := range pipelines {
		t.Run(filepath.Base(pipeline), func(t *testing.T) {
			called := makeInvocations(t, pipeline)
			require.NotEmpty(t, called,
				"%s runs no make target, so it is testing something else", pipeline)

			for _, target := range called {
				require.True(t, targets[target],
					"%s calls `make %s`, which the Makefile does not define",
					pipeline, target)
			}
		})
	}
}

// Both pipelines must run the same gates on both operating systems. macOS is
// where the developers are and Linux is where CI is; a project that only ever
// builds on one finds out about the other from a user.
func TestPipelinesBuildOnLinuxAndMacOS(t *testing.T) {
	for _, pipeline := range pipelines {
		t.Run(filepath.Base(pipeline), func(t *testing.T) {
			body, err := os.ReadFile(pipeline)
			require.NoError(t, err)

			text := strings.ToLower(string(body))
			require.Contains(t, text, "linux", "%s never names a Linux runner", pipeline)
			require.Contains(t, text, "macos", "%s never names a macOS runner", pipeline)
		})
	}
}

// The gates are the same set on both forges, or one of them is a weaker gate
// wearing the same badge.
func TestPipelinesRunTheSameGates(t *testing.T) {
	want := map[string][]string{}
	for _, pipeline := range pipelines {
		called := makeInvocations(t, pipeline)

		unique := map[string]bool{}
		for _, target := range called {
			unique[target] = true
		}

		sorted := make([]string, 0, len(unique))
		for target := range unique {
			sorted = append(sorted, target)
		}
		want[pipeline] = sorted
	}

	first := pipelines[0]
	for _, pipeline := range pipelines[1:] {
		require.ElementsMatch(t, want[first], want[pipeline],
			"%s and %s do not run the same make targets", first, pipeline)
	}
}
