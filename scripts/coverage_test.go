// Package scripts_test exercises the shell scripts that guard the build. They
// are the only part of the tree that CI runs and Go does not compile, so they
// get tests of their own rather than trust.
package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// runGate runs the coverage gate against one of the fixture modules in
// testdata/ and returns its combined output and exit code. The fixtures are
// whole modules, so the gate is exercised end to end — it runs their tests,
// reads the profile it produced and decides — rather than through a stub.
func runGate(t *testing.T, fixture string) (string, int) {
	t.Helper()

	script, err := filepath.Abs("coverage.sh")
	require.NoError(t, err)

	dir, err := filepath.Abs(filepath.Join("testdata", fixture))
	require.NoError(t, err)

	profile := filepath.Join(t.TempDir(), "coverage.out")

	cmd := exec.Command("sh", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "COVERAGE_PROFILE="+profile)

	out, err := cmd.CombinedOutput()
	t.Logf("coverage.sh in %s:\n%s", fixture, out)

	var exit *exec.ExitError
	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &exit):
		return string(out), exit.ExitCode()
	default:
		t.Fatalf("running the gate: %v", err)
		return "", -1
	}
}

func TestCoverageGateRejectsATreeBelowTheOverallFloor(t *testing.T) {
	out, code := runGate(t, "lowcoverage")

	require.NotZero(t, code, "a tree under 85%% overall must fail the gate")
	require.Contains(t, out, "85", "the failure must name the floor it missed")
}

// The overall floor and the internal/model floor are independent gates, and a
// tree can satisfy either while violating the other. This fixture clears 85%
// overall precisely because internal/model is small.
func TestCoverageGateRejectsAnUncoveredModelPackage(t *testing.T) {
	out, code := runGate(t, "modelgap")

	require.NotZero(t, code, "internal/model under 100%% must fail the gate")
	require.Contains(t, out, "internal/model", "the failure must name the package")
	require.NotContains(t, strings.ToLower(out), "skipped",
		"the package floor applies whenever the package exists")
}

// Until M1-S2 there is no internal/model, and a gate that fails on a package
// that does not exist yet is a gate nobody can land the first story past.
func TestCoverageGateSkipsTheModelFloorWhenThePackageIsAbsent(t *testing.T) {
	out, code := runGate(t, "nomodel")

	require.Zero(t, code, "a missing internal/model is skipped, not failed")
	require.Contains(t, strings.ToLower(out), "skipped",
		"the gate must say out loud that a floor did not apply")
}

func TestCoverageGateAcceptsATreeMeetingBothFloors(t *testing.T) {
	_, code := runGate(t, "allgreen")

	require.Zero(t, code, "a tree meeting both floors must pass the gate")
}
