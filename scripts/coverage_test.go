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
func runGate(t *testing.T, fixture string, env ...string) (string, int) {
	t.Helper()

	script, err := filepath.Abs("coverage.sh")
	require.NoError(t, err)

	dir, err := filepath.Abs(filepath.Join("testdata", fixture))
	require.NoError(t, err)

	profile := filepath.Join(t.TempDir(), "coverage.out")

	cmd := exec.Command("sh", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "COVERAGE_PROFILE="+profile)
	cmd.Env = append(cmd.Env, env...)

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

func TestCoverageGateRejectsATreeBelowTheProductFloor(t *testing.T) {
	out, code := runGate(t, "lowcoverage")

	require.NotZero(t, code, "a tree under 99%% across the product must fail the gate")
	require.Contains(t, out, "99", "the failure must name the floor it missed")
}

// The product floor and the internal/model floor are independent gates, and a
// tree can satisfy either while violating the other. The product floor is
// lowered here so that only the package floor can be what rejects this
// fixture — which is what the environment knobs are for, and the only way to
// prove the two are separate rather than one gate wearing two names.
func TestCoverageGateRejectsAnUncoveredModelPackage(t *testing.T) {
	out, code := runGate(t, "modelgap", "COVERAGE_MIN=80")

	require.NotZero(t, code, "internal/model under 100%% must fail the gate")
	require.Contains(t, out, "internal/model", "the failure must name the package")
	require.Contains(t, out, "80%", "the product floor was met, so it is not what failed")
	require.NotContains(t, modelLine(t, out), "skipped",
		"the package floor applies whenever the package exists")
}

// The harness carries a floor of its own, below the product's, because its
// uncovered lines are the t.Fatalf handlers whose whole job is to fail a test
// — see the note in coverage.sh. Below its own floor it is still rejected.
func TestCoverageGateRejectsAnUnderTestedHarness(t *testing.T) {
	out, code := runGate(t, "harnessgap", "COVERAGE_MIN=70")

	require.NotZero(t, code, "internal/gittest under 90%% must fail the gate")
	require.Contains(t, out, "internal/gittest", "the failure must name the package")
	require.Contains(t, out, "the harness", "and say what it is being held to")
}

// The harness's statements are not counted towards the product floor. A tree
// whose harness is perfect must not thereby clear a product floor its own code
// misses, and this fixture proves the two totals are separate: its 75% overall
// is 100% harness and 67% product, so a gate folding them together would let
// the product through at a floor it does not meet.
func TestTheHarnessDoesNotDiluteTheProductFloor(t *testing.T) {
	out, code := runGate(t, "harnessgap",
		"COVERAGE_MIN=70", "COVERAGE_HARNESS_PKG=calc", "COVERAGE_HARNESS_MIN=0")

	require.NotZero(t, code,
		"with the harness named as calc, what is left is internal/gittest at 67%%")
	require.Contains(t, out, "across the product")
}

// modelLine is the gate's line about internal/model, so that an assertion about
// that floor is not answered by a different floor's output.
func modelLine(t *testing.T, out string) string {
	t.Helper()

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "internal/model") {
			return line
		}
	}

	t.Fatalf("the gate said nothing about internal/model:\n%s", out)

	return ""
}

// internal/model is M3-S1's derivation package, not M1's — M1 builds
// internal/issue, which is the on-disk format. So this floor stays skipped for
// three more milestones, and a gate that failed on a package that does not
// exist yet is a gate nobody could land a story past.
func TestCoverageGateSkipsTheModelFloorWhenThePackageIsAbsent(t *testing.T) {
	out, code := runGate(t, "nomodel")

	require.Zero(t, code, "a missing internal/model is skipped, not failed")
	require.Contains(t, strings.ToLower(out), "skipped",
		"the gate must say out loud that a floor did not apply")
	require.Contains(t, out, "internal/model")
	require.Contains(t, out, "internal/gittest", "the harness floor is skipped the same way")
}

func TestCoverageGateAcceptsATreeMeetingBothFloors(t *testing.T) {
	_, code := runGate(t, "allgreen")

	require.Zero(t, code, "a tree meeting both floors must pass the gate")
}
