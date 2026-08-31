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

.PHONY: all help build vet lint fmt test stress cover tools clean

all: build vet lint test cover

help:
	@echo 'build   compile every package'
	@echo 'vet     go vet ./...'
	@echo 'lint    golangci-lint, including the formatters'
	@echo 'fmt     rewrite files to satisfy the formatters'
	@echo 'test    go test ./...'
	@echo 'stress  the build-tagged races, which are out of the default suite'
	@echo 'cover   run the tests and enforce both coverage floors'
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

# The races M4-S4 keeps out of the default suite. Repeating a network operation
# a hundred times per CI run buys confidence in the network and not in the code,
# so the deterministic test is the gate and this is what you run when you do not
# believe it.
stress:
	$(GO) test -tags stress -run 'Stress|Claimants' -count 1 ./...

cover:
	COVERAGE_PROFILE=$(COVERAGE_PROFILE) sh scripts/coverage.sh

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
