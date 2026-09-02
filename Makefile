# Every gate in this project is a make target, and CI calls these and nothing
# else — see M0-S3. Logic that lives in a pipeline file can only be run by that
# pipeline, so a contributor cannot run the gates before pushing and the
# pipeline becomes the only thing that knows whether the tree is green.

GO ?= go
GOBIN ?= $(shell $(GO) env GOPATH)/bin
GOLANGCI_LINT_VERSION ?= v2.5.0

# Prefer a golangci-lint already on PATH; fall back to the one `make tools`
# installs, so CI needs no PATH surgery of its own.
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo $(GOBIN)/golangci-lint)

BINARY ?= isu
COVERAGE_PROFILE ?= coverage.out

.PHONY: all help build vet lint fmt test perf stress cover dogfood tools clean

all: build vet lint test perf cover dogfood

help:
	@echo 'build   compile every package'
	@echo 'vet     go vet ./...'
	@echo 'lint    golangci-lint, including the formatters'
	@echo 'fmt     rewrite files to satisfy the formatters'
	@echo 'test    go test ./...'
	@echo 'perf    M2-S5 wall-clock gate, measured with the machine to itself'
	@echo 'stress  the build-tagged races, which are out of the default suite'
	@echo 'cover   run the tests and enforce both coverage floors'
	@echo 'dogfood run isu check over this repository, with the isu just built'
	@echo 'tools   install the pinned golangci-lint'
	@echo 'clean   remove build and coverage output'

build:
	$(GO) build -o $(BINARY) ./cmd/isu
	$(GO) build ./...

vet:
	$(GO) vet ./...

lint:
	$(GOLANGCI_LINT) run

fmt:
	$(GOLANGCI_LINT) fmt

test:
	$(GO) test ./...

# The wall-clock half of M2-S5's gate, and the only place it is asserted.
#
# A budget on elapsed time is a claim about the whole machine, so this measures
# with the machine to itself: one package, one test process, nothing else in
# flight. `go test ./...` still runs these tests, and still logs the number they
# measure — what it does not do is hold a figure taken beside seventy seconds of
# somebody else's git to a budget calibrated without it. The process
# counts, which are what actually prevent the regression, are asserted in every
# pass either way.
#
# -count 1 because a timing measurement is never the cached one, and -v because
# `go test` hides a passing test's log: without it this step records that the
# budgets held but not what they held, and a gate whose number nobody can see is
# one nobody will notice drifting until it fails.
perf:
	ISU_PERF=1 $(GO) test -count 1 -p 1 -v -run IsFast ./internal/repo/

# The races M4-S4 keeps out of the default suite. Repeating a network operation
# a hundred times per CI run buys confidence in the network and not in the code,
# so the deterministic test is the gate and this is what you run when you do not
# believe it.
stress:
	$(GO) test -tags stress -run 'Stress|Claimants' -count 1 ./...

cover:
	COVERAGE_PROFILE=$(COVERAGE_PROFILE) sh scripts/coverage.sh

# M5-S7: isu tracks its own construction, and this is where its own pipeline
# enforces that. The binary is the one just built rather than a release: a gate
# that checked this repository with last month's isu would pass the pull request
# that broke the checks.
#
# ISU_CHECK_ARGS is how CI names the base branch. A pull request build checks
# out a merge commit and no branch, so without --ref isu compares the repository
# against itself and every rule about the branch passes silently.
ISU_CHECK_ARGS ?=

dogfood: build
	./$(BINARY) check $(ISU_CHECK_ARGS)

# golangci-lint v2.5.0 needs go >= 1.24.0. The go directive in go.mod is 1.24.0
# for exactly this reason, so `go install` builds it with the toolchain already
# installed and never switches. When the directive was 1.23.0 it did switch —
# to whatever Go had released most recently, resolved fresh on every run — and
# the gate went red the day one of those releases arrived incomplete. Raising
# the directive is what removed that; nothing here pins a toolchain.
tools:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

clean:
	rm -f $(BINARY) $(COVERAGE_PROFILE) coverage.html
	$(GO) clean -testcache
