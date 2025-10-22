LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
GOLANGCILINT_VERSION ?= 1.60.1
GLOBAL_GOLANGCILINT := $(shell which golangci-lint)
GOBIN_GOLANGCILINT:= $(shell which $(GOBIN)/golangci-lint)
ENVTEST_K8S_VERSION = 1.31.0
ENVTEST ?= $(LOCALBIN)/setup-envtest

.PHONY: golangci
golangci:
ifeq ($(shell $(GLOBAL_GOLANGCILINT) version --format short), $(GOLANGCILINT_VERSION))
	@$(OK) golangci-lint is already installed
GOLANGCILINT=$(GLOBAL_GOLANGCILINT)
else ifeq ($(shell $(GOBIN_GOLANGCILINT) version --format short), $(GOLANGCILINT_VERSION))
	@$(OK) golangci-lint is already installed
GOLANGCILINT=$(GOBIN_GOLANGCILINT)
else
	@{ \
	set -e ;\
	echo 'installing golangci-lint-$(GOLANGCILINT_VERSION)' ;\
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) v$(GOLANGCILINT_VERSION) ;\
	echo 'Successfully installed' ;\
	}
GOLANGCILINT=$(GOBIN)/golangci-lint
endif

.PHONY: staticchecktool
staticchecktool:
ifeq (, $(shell which staticcheck))
	@{ \
	set -e ;\
	echo 'installing honnef.co/go/tools/cmd/staticcheck ' ;\
	go install honnef.co/go/tools/cmd/staticcheck@v0.6.1 ;\
	}
STATICCHECK=$(GOBIN)/staticcheck
else
STATICCHECK=$(shell which staticcheck)
endif

.PHONY: goimports
goimports:
ifeq (, $(shell which goimports))
	@{ \
	set -e ;\
	go install golang.org/x/tools/cmd/goimports@6546d82b229aa5bd9ebcc38b09587462e34b48b6 ;\
	}
GOIMPORTS=$(GOBIN)/goimports
else
GOIMPORTS=$(shell which goimports)
endif

CUE_VERSION ?= v0.14.1
.PHONY: installcue
installcue:
ifeq (, $(shell which cue))
	@{ \
	set -e ;\
	go install cuelang.org/go/cmd/cue@$(CUE_VERSION) ;\
	}
CUE=$(GOBIN)/cue
else
CUE=$(shell which cue)
endif

KUSTOMIZE_VERSION ?= 4.5.4
KUSTOMIZE = $(shell pwd)/bin/kustomize
.PHONY: kustomize
kustomize:
ifneq (, $(shell kustomize version | grep $(KUSTOMIZE_VERSION)))
KUSTOMIZE=$(shell which kustomize)
else ifneq (, $(shell $(KUSTOMIZE) version | grep $(KUSTOMIZE_VERSION)))
else
	@{ \
	set -eo pipefail ;\
    echo "installing kustomize-v$(KUSTOMIZE_VERSION) into $(shell pwd)/bin" ;\
    mkdir -p $(shell pwd)/bin ;\
    rm -f $(KUSTOMIZE) ;\
	curl -sS https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh | bash -s $(KUSTOMIZE_VERSION) $(shell pwd)/bin;\
	echo 'Install succeed' ;\
    }
endif

.PHONY: helmdoc
helmdoc:
ifeq (, $(shell which readme-generator))
	@{ \
	set -e ;\
	echo 'installing readme-generator-for-helm' ;\
	npm install -g @bitnami/readme-generator-for-helm ;\
	}
else
	@$(OK) readme-generator-for-helm is already installed
HELMDOC=$(shell which readme-generator)
endif

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: sync-crds
PKG_MODULE = github.com/kubevela/pkg # fetch common crds from the pkg repo instead of generating locally
sync-crds: ## Copy CRD from pinned module version in go.mod
	@moddir=$$(go list -m -f '{{.Dir}}' $(PKG_MODULE) 2>/dev/null); \
	mkdir -p config/crd/base; \
	for file in $(COMMON_CRD_FILES); do \
		src="$$moddir/crds/$$file"; \
		cp -f "$$src" "config/crd/base/"; \
	done