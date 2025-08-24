LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
GOLANGCILINT_VERSION ?= 1.60.1
GLOBAL_GOLANGCILINT := $(shell which golangci-lint)
GOBIN_GOLANGCILINT:= $(shell which $(GOBIN)/golangci-lint)
ENVTEST_K8S_VERSION = 1.29.0
ENVTEST ?= $(LOCALBIN)/setup-envtest

.PHONY: setup-envtest
setup-envtest: $(ENVTEST)
	@$(INFO) "Downloading envtest binaries"
	@mkdir -p /usr/local/kubebuilder/bin || sudo mkdir -p /usr/local/kubebuilder/bin
	@chmod -R 777 /usr/local/kubebuilder/bin 2>/dev/null || sudo chmod -R 777 /usr/local/kubebuilder/bin
	@KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" && \
	echo "Copying binaries from $$KUBEBUILDER_ASSETS to /usr/local/kubebuilder/bin" && \
	if [ -d "$$KUBEBUILDER_ASSETS" ]; then \
		find "$$KUBEBUILDER_ASSETS" -type f -exec cp {} /usr/local/kubebuilder/bin/ \; || \
		sudo find "$$KUBEBUILDER_ASSETS" -type f -exec cp {} /usr/local/kubebuilder/bin/ \; ; \
	else \
		echo "ERROR: KUBEBUILDER_ASSETS directory not found"; \
		exit 1; \
	fi
	@chmod -R +x /usr/local/kubebuilder/bin/* 2>/dev/null || sudo chmod -R +x /usr/local/kubebuilder/bin/*
	@ls -la /usr/local/kubebuilder/bin/
	@if [ ! -f "/usr/local/kubebuilder/bin/etcd" ]; then \
		$(MAKE) download-etcd; \
	fi
	@$(OK) "Envtest binaries downloaded to /usr/local/kubebuilder/bin"

.PHONY: download-etcd
download-etcd:
	@$(INFO) "Directly downloading etcd"
	@mkdir -p /tmp/etcd-download
	@curl -L --retry 5 --retry-delay 3 https://github.com/etcd-io/etcd/releases/download/v3.5.7/etcd-v3.5.7-linux-amd64.tar.gz -o /tmp/etcd.tar.gz
	@tar -xzf /tmp/etcd.tar.gz -C /tmp/etcd-download --strip-components=1
	@cp /tmp/etcd-download/etcd /usr/local/kubebuilder/bin/ 2>/dev/null || sudo cp /tmp/etcd-download/etcd /usr/local/kubebuilder/bin/
	@cp /tmp/etcd-download/etcdctl /usr/local/kubebuilder/bin/ 2>/dev/null || sudo cp /tmp/etcd-download/etcdctl /usr/local/kubebuilder/bin/
	@chmod +x /usr/local/kubebuilder/bin/etcd 2>/dev/null || sudo chmod +x /usr/local/kubebuilder/bin/etcd
	@chmod +x /usr/local/kubebuilder/bin/etcdctl 2>/dev/null || sudo chmod +x /usr/local/kubebuilder/bin/etcdctl
	@rm -rf /tmp/etcd.tar.gz /tmp/etcd-download
	@$(OK) "Etcd installed successfully at /usr/local/kubebuilder/bin/etcd"

.PHONY: download-kube-apiserver
download-kube-apiserver:
	@$(INFO) "Directly downloading kube-apiserver"
	@curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v1.26.1/bin/linux/amd64/kube-apiserver -o /usr/local/kubebuilder/bin/kube-apiserver 2>/dev/null || \
		sudo curl -L --retry 5 --retry-delay 3 https://dl.k8s.io/v1.26.1/bin/linux/amd64/kube-apiserver -o /usr/local/kubebuilder/bin/kube-apiserver
	@chmod +x /usr/local/kubebuilder/bin/kube-apiserver 2>/dev/null || sudo chmod +x /usr/local/kubebuilder/bin/kube-apiserver
	@$(OK) "kube-apiserver installed successfully at /usr/local/kubebuilder/bin/kube-apiserver"

.PHONY: verify-test-binaries
verify-test-binaries:
	@$(INFO) "Verifying test binaries"
	@if [ ! -f "/usr/local/kubebuilder/bin/etcd" ]; then \
		echo "etcd not found, downloading..."; \
		$(MAKE) download-etcd; \
	else \
		echo "✓ etcd found at /usr/local/kubebuilder/bin/etcd"; \
	fi
	@if [ ! -f "/usr/local/kubebuilder/bin/kube-apiserver" ]; then \
		echo "kube-apiserver not found, downloading..."; \
		$(MAKE) download-kube-apiserver; \
	else \
		echo "✓ kube-apiserver found at /usr/local/kubebuilder/bin/kube-apiserver"; \
	fi
	@$(OK) "Test binaries verified"

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
	go install honnef.co/go/tools/cmd/staticcheck@v0.5.1 ;\
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

CUE_VERSION ?= v0.9.2
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
	@{ \
	if which kustomize > /dev/null 2>&1 && kustomize version 2>&1 | grep -q $(KUSTOMIZE_VERSION); then \
		echo "✓ kustomize version $(KUSTOMIZE_VERSION) is already installed"; \
		KUSTOMIZE=$$(which kustomize); \
	elif [ -f "$(KUSTOMIZE)" ] && "$(KUSTOMIZE)" version 2>&1 | grep -q $(KUSTOMIZE_VERSION); then \
		echo "✓ kustomize version $(KUSTOMIZE_VERSION) is already installed at $(KUSTOMIZE)"; \
	else \
		echo "Installing kustomize-v$(KUSTOMIZE_VERSION) into $(shell pwd)/bin"; \
		mkdir -p $(shell pwd)/bin; \
		rm -f $(KUSTOMIZE); \
		curl -sS --retry 5 --retry-delay 3 https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh | bash -s $(KUSTOMIZE_VERSION) $(shell pwd)/bin || { echo "Failed to install kustomize"; exit 1; }; \
		chmod +x $(KUSTOMIZE); \
		echo "✓ kustomize installed successfully"; \
	fi; \
	}

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
