#!/bin/sh
# coverage.sh — the coverage gate from the definition of done.
#
# Three floors, none able to hide behind another:
#
#   * 99% across the product — every package except the test harness;
#   * 100% for internal/model, which is the product's core;
#   * 90% for internal/gittest, which is the harness.
#
# **Why the harness is counted separately rather than with everything else.**
# internal/gittest exists to serve tests: it builds git repositories and fails
# the test when it cannot. Its uncovered lines are almost entirely the t.Fatalf
# handlers that do that failing, and driving them means handing it a counterfeit
# testing.TB — machinery built for no reason except to move this number, which
# is a gate producing worse engineering rather than better. So the harness gets
# a floor of its own, high enough that it cannot rot and low enough that nobody
# is tempted. It is not excused, and it is not allowed to dilute the product's
# floor either: at 267 statements it is a fifth of the tree, so folding it in
# would cost the product roughly a point of headroom it should be spending on
# code that ships.
#
# Run from a module root. Overridable through the environment so that the
# gate's own tests can point it at fixture modules:
#
#   COVERAGE_PROFILE        where to write the profile      (coverage.out)
#   COVERAGE_MIN            the product floor               (99)
#   COVERAGE_FLOOR_PKG      the package with its own floor  (internal/model)
#   COVERAGE_FLOOR_PKG_MIN  that package's floor            (100)
#   COVERAGE_HARNESS_PKG    the harness, floored separately (internal/gittest)
#   COVERAGE_HARNESS_MIN    the harness's floor             (90)
set -eu

PROFILE=${COVERAGE_PROFILE:-coverage.out}
MIN_TOTAL=${COVERAGE_MIN:-99}
FLOOR_PKG=${COVERAGE_FLOOR_PKG:-internal/model}
MIN_FLOOR_PKG=${COVERAGE_FLOOR_PKG_MIN:-100}
HARNESS_PKG=${COVERAGE_HARNESS_PKG:-internal/gittest}
MIN_HARNESS=${COVERAGE_HARNESS_MIN:-90}

# -coverpkg=./... is what makes this the gate §0 asks for rather than a
# flattering subset of it: without it, a package with no test file of its own
# is simply absent from the profile, so leaving a package untested raises the
# reported average instead of lowering it.
go test ./... -coverpkg=./... -covermode=count -coverprofile="$PROFILE"

# Each test binary reports every instrumented block, so a block appears once
# per binary that ran and the counts have to be folded before they are read.
# Comparisons stay in integers — covered*100 >= floor*total — because 98.999%
# rounds to 99.0% and a gate that rounds in the tree's favour is not a gate.
eval "$(awk -v pkg="/$FLOOR_PKG/" -v harness="/$HARNESS_PKG/" \
	-v minTotal="$MIN_TOTAL" -v minPkg="$MIN_FLOOR_PKG" -v minHarness="$MIN_HARNESS" '
	NR == 1 { next }   # the mode: line
	{
		if (!($1 in stmts)) stmts[$1] = $2
		hits[$1] += $3
	}
	END {
		for (block in stmts) {
			n = stmts[block]
			covered = (hits[block] > 0)

			if (index(block, harness) > 0) {
				harnessTotal += n
				if (covered) harnessCovered += n
			} else {
				total += n
				if (covered) product += n
			}

			if (index(block, pkg) > 0) {
				pkgTotal += n
				if (covered) pkgCovered += n
			}
		}
		printf "total_stmts=%d\n", total
		printf "pkg_stmts=%d\n", pkgTotal
		printf "harness_stmts=%d\n", harnessTotal
		printf "total_pct=%.1f\n", (total ? product * 100 / total : 100)
		printf "pkg_pct=%.1f\n", (pkgTotal ? pkgCovered * 100 / pkgTotal : 100)
		printf "harness_pct=%.1f\n", (harnessTotal ? harnessCovered * 100 / harnessTotal : 100)
		printf "total_ok=%d\n", (product * 100 >= minTotal * total ? 1 : 0)
		printf "pkg_ok=%d\n", (pkgCovered * 100 >= minPkg * pkgTotal ? 1 : 0)
		printf "harness_ok=%d\n", (harnessCovered * 100 >= minHarness * harnessTotal ? 1 : 0)
	}
' "$PROFILE")"

status=0

printf 'coverage: %s%% of %s statements across the product (floor %s%%)\n' \
	"$total_pct" "$total_stmts" "$MIN_TOTAL"
if [ "$total_ok" -ne 1 ]; then
	printf 'FAIL: product coverage %s%% is below the %s%% floor\n' \
		"$total_pct" "$MIN_TOTAL" >&2
	status=1
fi

if [ -d "$FLOOR_PKG" ]; then
	printf 'coverage: %s%% of %s statements in %s (floor %s%%)\n' \
		"$pkg_pct" "$pkg_stmts" "$FLOOR_PKG" "$MIN_FLOOR_PKG"
	if [ "$pkg_ok" -ne 1 ]; then
		printf 'FAIL: %s coverage %s%% is below the %s%% floor\n' \
			"$FLOOR_PKG" "$pkg_pct" "$MIN_FLOOR_PKG" >&2
		status=1
	fi
else
	printf 'skipped: no %s in this tree, so its %s%% floor does not apply yet\n' \
		"$FLOOR_PKG" "$MIN_FLOOR_PKG"
fi

if [ -d "$HARNESS_PKG" ]; then
	printf 'coverage: %s%% of %s statements in %s, the harness (floor %s%%)\n' \
		"$harness_pct" "$harness_stmts" "$HARNESS_PKG" "$MIN_HARNESS"
	if [ "$harness_ok" -ne 1 ]; then
		printf 'FAIL: %s coverage %s%% is below the %s%% floor\n' \
			"$HARNESS_PKG" "$harness_pct" "$MIN_HARNESS" >&2
		status=1
	fi
else
	printf 'skipped: no %s in this tree, so its %s%% floor does not apply yet\n' \
		"$HARNESS_PKG" "$MIN_HARNESS"
fi

exit "$status"
