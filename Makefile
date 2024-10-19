
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

GO = go
GOLANGCI-LINT = golangci-lint
KUBECTL = kubectl

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: fmt vet test
fmt vet test:
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

.PHONY: generate
generate: api
	@$(GO) $@ ./...

.PHONY: manifests
manifests: controller-gen
	@$(CONTROLLER_GEN) crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: api
api: controller-gen
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: config
config: manifests

.PHONY: install
install:
	@$(KUBECTL) kustomize config/crd | $(KUBECTL) apply -f-

.PHONY: uninstall
uninstall:
	@$(KUBECTL) kustomize config/crd | $(KUBECTL) delete -f-

.PHONY: run
run: api config install fmt vet
	@$(GO) $@ ./

ifndef ignore-not-found
  ignore-not-found = false
endif

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	@mkdir -p $(LOCALBIN)

CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

CONTROLLER_TOOLS_VERSION ?= v0.16.4

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(CONTROLLER_GEN) && $(CONTROLLER_GEN) --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
		GOBIN=$(LOCALBIN) $(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
