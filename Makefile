
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

GO = go
GOLANGCI-LINT = golangci-lint

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: fmt vet generate test
fmt vet generate test:
	@$(GO) $@ ./...

.PHONY: download vendor verify
download vendor verify:
	@$(GO) mod $@

.PHONY: lint
lint: fmt
	@$(GOLANGCI-LINT) run --fix

.PHONY: gen dl ven ver format
gen: generate
dl: download
ven: vendor
ver: verify
format: fmt

.PHONY: config
config: $(CONTROLLER_GEN)
	@$(CONTROLLER_GEN) rbac:roleName=manager-role webhook paths="./..."

.PHONY: run
run: config fmt vet
	@$(GO) $@ ./

ifndef ignore-not-found
  ignore-not-found = false
endif

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

CONTROLLER_TOOLS_VERSION ?= v0.11.1

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(CONTROLLER_GEN) && $(CONTROLLER_GEN) --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
		GOBIN=$(LOCALBIN) $(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
