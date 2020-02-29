GO              ?= $(shell which go)

GO_BUILD_SRC    ?= $(shell find $(CURDIR) -type f -name \*.go)
GO_BUILD_TARGET ?= modularise
GO_BUILD_FLAGS  ?=
GO_TEST_TARGET  ?= .go-test
GO_TEST_FLAGS   ?=

# -- default targets ----------------------------------------------------------

.PHONY: all build test

all: build

build: go-build

test: go-test

# -- go -----------------------------------------------------------------------

.PHONY: go-build go-test

go-build: $(GO_BUILD_TARGET)

go-test: $(GO_TEST_TARGET)

$(GO_BUILD_TARGET): $(GO_BUILD_SRC)
	$(GO) build $(GO_BUILD_FLAGS) -o $@

$(GO_TEST_TARGET): $(GO_BUILD_SRC)
	$(GO) test $(GO_TEST_FLAGS) ./...
	@touch $@
