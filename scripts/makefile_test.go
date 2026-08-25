package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pipeline is the CI definition this repository ships. It is allowed to say
// where the gates run and on what; the gates themselves are make targets, so
// that what CI runs and what a developer runs are the same commands.
var pipeline = filepath.Join("..", ".github", "workflows", "ci.yml")

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
	require.NoError(t, err, "the Makefile is what the pipeline calls")

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

// TestMakefileTargetsExist keeps the pipeline and the Makefile in step: CI may
// only name targets that exist, which is also what stops a gate being defined
// in YAML where nobody can run it before pushing.
func TestMakefileTargetsExist(t *testing.T) {
	targets := makeTargets(t)

	called := makeInvocations(t, pipeline)
	require.NotEmpty(t, called,
		"%s runs no make target, so it is testing something else", pipeline)

	for _, target := range called {
		require.True(t, targets[target],
			"%s calls `make %s`, which the Makefile does not define", pipeline, target)
	}
}

// The gates run on both operating systems. macOS is where the developers are
// and Linux is where CI is; a project that only ever builds on one finds out
// about the other from a user.
func TestThePipelineBuildsOnLinuxAndMacOS(t *testing.T) {
	body, err := os.ReadFile(pipeline)
	require.NoError(t, err)

	text := strings.ToLower(string(body))
	require.Contains(t, text, "linux", "%s never names a Linux runner", pipeline)
	require.Contains(t, text, "macos", "%s never names a macOS runner", pipeline)
}
