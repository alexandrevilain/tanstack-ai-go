# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CONTAINER_TOOL ?= docker
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: generate mockery
generate:  ## Generate code.
	$(MOCKERY)
	go generate ./...

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: generate fmt vet ## Run tests.
	go test -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ License

.PHONY: ensure-license
ensure-license: go-licenser ## Run go-licenser to ensure license header is present on all files.
	$(GO_LICENSER) -licensor "Alexandre VILAIN" -license ASL2 .

.PHONY: check-license
check-license: go-licenser ## Run go-licenser to check that license header is present on all files.
	$(GO_LICENSER) -licensor "Alexandre VILAIN" -license ASL2 -d .

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GO_LICENSER = $(LOCALBIN)/go-licenser
MOCKERY = $(LOCALBIN)/mockery

## Tool Versions
GOLANGCI_LINT_VERSION ?= v2.10.1
GO_LICENSER_VERSION ?= v0.4.2
MOCKERY_VERSION ?= v3.6.4

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: go-licenser
go-licenser: $(GO_LICENSER) ## Download go-licencer locally if necessary.
$(GO_LICENSER): $(LOCALBIN)
	$(call go-install-tool,$(GO_LICENSER),github.com/elastic/go-licenser,$(GO_LICENSER_VERSION))

.PHONY: mockery
mockery: $(MOCKERY) ## Download go-licencer locally if necessary.
$(MOCKERY): $(LOCALBIN)
	$(call go-install-tool,$(MOCKERY),github.com/vektra/mockery/v3,$(MOCKERY_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef