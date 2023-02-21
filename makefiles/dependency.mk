
GOLANGCILINT_VERSION ?= 1.49.0
GLOBAL_GOLANGCILINT := $(shell which golangci-lint)
GOBIN_GOLANGCILINT:= $(shell which $(GOBIN)/golangci-lint)

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
	go install honnef.co/go/tools/cmd/staticcheck@d7e217c1ff411395475b2971c0824e1e7cc1af98 ;\
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

.PHONY: installcue
installcue:
ifeq (, $(shell which cue))
	@{ \
	set -e ;\
	go install cuelang.org/go/cmd/cue@latest ;\
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