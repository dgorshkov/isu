# Every gate in this project is a make target, and CI on both forges calls
# these and nothing else — see M0-S3. Logic that lives in a pipeline file can
# only be run by that pipeline, and then only one of the two forges is honest.

GO ?= go
GOBIN ?= $(shell $(GO) env GOPATH)/bin
GOLANGCI_LINT_VERSION ?= v2.5.0

# Prefer a golangci-lint already on PATH; fall back to the one `make tools`
# installs, so CI needs no PATH surgery of its own.
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || echo $(GOBIN)/golangci-lint)

BINARY ?= isu
COVERAGE_PROFILE ?= coverage.out

.PHONY: all help build vet lint fmt test cover tools clean

all: build vet lint test cover

help:
	@echo 'build   compile every package'
	@echo 'vet     go vet ./...'
	@echo 'lint    golangci-lint, including the formatters'
	@echo 'fmt     rewrite files to satisfy the formatters'
	@echo 'test    go test ./...'
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

cover:
	COVERAGE_PROFILE=$(COVERAGE_PROFILE) sh scripts/coverage.sh

tools:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

clean:
	rm -f $(BINARY) $(COVERAGE_PROFILE) coverage.html
	$(GO) clean -testcache
