GO                      ?= $(shell which go)
GOLANGCI_LINT           ?= $(shell which golangci-lint 2>/dev/null || echo $(shell $(GO) env GOPATH)/bin/golangci-lint)

GO_BUILD_SRC            ?= $(shell find $(CURDIR) -type f -name \*.go)
GO_BUILD_TARGET         ?= modularise
GO_BUILD_FLAGS          ?=
GO_TEST_TARGET          ?= .go-test
GO_TEST_FLAGS           ?=
GO_GOLANGCI_LINT_TARGET ?= .go-golangci-lint

# -- default targets ----------------------------------------------------------

.PHONY: all build test

all: build

build: go-build

test: go-test

# -- go -----------------------------------------------------------------------

.PHONY: go-build go-test

go-build: $(GO_BUILD_TARGET)

go-test: $(GO_GOLANGCI_LINT_TARGET) $(GO_TEST_TARGET)

$(GO_BUILD_TARGET): $(GO_BUILD_SRC)
	$(GO) build $(GO_BUILD_FLAGS) -o $@

$(GO_TEST_TARGET): $(GO_BUILD_SRC)
	$(GO) test $(GO_TEST_FLAGS) ./...
	@touch $@

$(GO_GOLANGCI_LINT_TARGET): $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run
	@touch $@

$(GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell $(GO) env GOPATH)/bin v1.23.7
