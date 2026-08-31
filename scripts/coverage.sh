#!/bin/sh
# coverage.sh — the coverage gate from the definition of done.
#
# Two floors, both enforced here and neither able to hide behind the other:
#
#   * 99% across the whole module, cmd/ and the test harness included;
#   * 100% for internal/model.
#
# **99% counts everything, internal/gittest included, and that is deliberate.**
# The harness fails the test rather than returning an error, so its refusals
# cannot be exercised the ordinary way — calling one from a test fails that
# test. They are driven instead through a counterfeit testing.TB that records
# the refusal and stops (internal/gittest/fatal_test.go). That is machinery, and
# the argument for exempting the harness instead is that it is machinery built
# to move a number. The argument against, which won: a harness nobody has ever
# seen refuse is a harness whose refusals are a comment, and every fixture in
# this project trusts them.
#
# Run from a module root. Overridable through the environment so that the
# gate's own tests can point it at fixture modules:
#
#   COVERAGE_PROFILE       where to write the profile   (coverage.out)
#   COVERAGE_MIN           the overall floor            (99)
#   COVERAGE_FLOOR_PKG     the package with its own floor (internal/model)
#   COVERAGE_FLOOR_PKG_MIN that package's floor          (100)
set -eu

PROFILE=${COVERAGE_PROFILE:-coverage.out}
MIN_TOTAL=${COVERAGE_MIN:-99}
FLOOR_PKG=${COVERAGE_FLOOR_PKG:-internal/model}
MIN_FLOOR_PKG=${COVERAGE_FLOOR_PKG_MIN:-100}

# -coverpkg=./... is what makes this the gate §0 asks for rather than a
# flattering subset of it: without it, a package with no test file of its own
# is simply absent from the profile, so leaving a package untested raises the
# reported average instead of lowering it.
go test ./... -coverpkg=./... -covermode=count -coverprofile="$PROFILE"

# Each test binary reports every instrumented block, so a block appears once
# per binary that ran and the counts have to be folded before they are read.
# Comparisons stay in integers — covered*100 >= floor*total — because 98.999%
# rounds to 99.0% and a gate that rounds in the tree's favour is not a gate.
eval "$(awk -v pkg="/$FLOOR_PKG/" -v minTotal="$MIN_TOTAL" -v minPkg="$MIN_FLOOR_PKG" '
	NR == 1 { next }   # the mode: line
	{
		if (!($1 in stmts)) stmts[$1] = $2
		hits[$1] += $3
	}
	END {
		for (block in stmts) {
			n = stmts[block]
			total += n
			if (hits[block] > 0) covered += n
			if (index(block, pkg) > 0) {
				pkgTotal += n
				if (hits[block] > 0) pkgCovered += n
			}
		}
		printf "total_stmts=%d\n", total
		printf "pkg_stmts=%d\n", pkgTotal
		printf "total_pct=%.1f\n", (total ? covered * 100 / total : 100)
		printf "pkg_pct=%.1f\n", (pkgTotal ? pkgCovered * 100 / pkgTotal : 100)
		printf "total_ok=%d\n", (covered * 100 >= minTotal * total ? 1 : 0)
		printf "pkg_ok=%d\n", (pkgCovered * 100 >= minPkg * pkgTotal ? 1 : 0)
	}
' "$PROFILE")"

status=0

printf 'coverage: %s%% of %s statements overall (floor %s%%)\n' \
	"$total_pct" "$total_stmts" "$MIN_TOTAL"
if [ "$total_ok" -ne 1 ]; then
	printf 'FAIL: overall coverage %s%% is below the %s%% floor\n' \
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

exit "$status"
