SHELL := /bin/bash

# Vela version
VELA_VERSION ?= master
# Repo info
GIT_COMMIT          ?= git-$(shell git rev-parse --short HEAD)
GIT_COMMIT_LONG     ?= $(shell git rev-parse HEAD)
VELA_VERSION_KEY    := github.com/oam-dev/kubevela/version.VelaVersion
VELA_GITVERSION_KEY := github.com/oam-dev/kubevela/version.GitRevision
LDFLAGS             ?= "-s -w -X $(VELA_VERSION_KEY)=$(VELA_VERSION) -X $(VELA_GITVERSION_KEY)=$(GIT_COMMIT)"

GOBUILD_ENV = GO111MODULE=on CGO_ENABLED=0
GOX         = go run github.com/mitchellh/gox
TARGETS     := darwin/amd64 linux/amd64 windows/amd64
DIST_DIRS   := find * -type d -exec

TIME_LONG	= `date +%Y-%m-%d' '%H:%M:%S`
TIME_SHORT	= `date +%H:%M:%S`
TIME		= $(TIME_SHORT)

BLUE         := $(shell printf "\033[34m")
YELLOW       := $(shell printf "\033[33m")
RED          := $(shell printf "\033[31m")
GREEN        := $(shell printf "\033[32m")
CNone        := $(shell printf "\033[0m")

INFO	= echo ${TIME} ${BLUE}[ .. ]${CNone}
WARN	= echo ${TIME} ${YELLOW}[WARN]${CNone}
ERR		= echo ${TIME} ${RED}[FAIL]${CNone}
OK		= echo ${TIME} ${GREEN}[ OK ]${CNone}
FAIL	= (echo ${TIME} ${RED}[FAIL]${CNone} && false)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Image URL to use all building/pushing image targets
VELA_CORE_IMAGE      ?= vela-core:latest
VELA_CORE_TEST_IMAGE ?= vela-core-test:$(GIT_COMMIT)
VELA_APISERVER_IMAGE      ?= apiserver:latest
VELA_RUNTIME_ROLLOUT_IMAGE       ?= vela-runtime-rollout:latest
VELA_RUNTIME_ROLLOUT_TEST_IMAGE  ?= vela-runtime-rollout-test:$(GIT_COMMIT)
RUNTIME_CLUSTER_CONFIG ?= /tmp/worker.kubeconfig
RUNTIME_CLUSTER_NAME ?= worker

all: build

# Run tests
test: vet lint staticcheck unit-test-core
	@$(OK) unit-tests pass

unit-test-core:
	go test -coverprofile=coverage.txt $(shell go list ./pkg/... ./cmd/... | grep -v apiserver)
	go test $(shell go list ./references/... | grep -v apiserver)
unit-test-apiserver:
	go test -coverprofile=coverage.txt $(shell go list ./pkg/... ./cmd/...  | grep -E 'apiserver|velaql')

# Build vela cli binary
build: fmt vet lint staticcheck vela-cli kubectl-vela
	@$(OK) build succeed

vela-cli:
	$(GOBUILD_ENV) go build -o bin/vela -a -ldflags $(LDFLAGS) ./references/cmd/cli/main.go

kubectl-vela:
	$(GOBUILD_ENV) go build -o bin/kubectl-vela -a -ldflags $(LDFLAGS) ./cmd/plugin/main.go

doc-gen:
	rm -r docs/en/cli/*
	go run hack/docgen/gen.go

cross-build:
	rm -rf _bin
	go get github.com/mitchellh/gox@v0.4.0
	$(GOBUILD_ENV) $(GOX) -ldflags $(LDFLAGS) -parallel=2 -output="_bin/vela/{{.OS}}-{{.Arch}}/vela" -osarch='$(TARGETS)' ./references/cmd/cli
	$(GOBUILD_ENV) $(GOX) -ldflags $(LDFLAGS) -parallel=2 -output="_bin/kubectl-vela/{{.OS}}-{{.Arch}}/kubectl-vela" -osarch='$(TARGETS)' ./cmd/plugin
	$(GOBUILD_ENV) $(GOX) -ldflags $(LDFLAGS) -parallel=2 -output="_bin/apiserver/{{.OS}}-{{.Arch}}/apiserver" -osarch="$(TARGETS)" ./cmd/apiserver

build-cleanup:
	rm -rf _bin

compress:
	( \
		echo "\n## Release Info\nVERSION: $(VELA_VERSION)" >> README.md && \
		echo "GIT_COMMIT: $(GIT_COMMIT_LONG)\n" >> README.md && \
		cd _bin/vela && \
		$(DIST_DIRS) cp ../../LICENSE {} \; && \
		$(DIST_DIRS) cp ../../README.md {} \; && \
		$(DIST_DIRS) tar -zcf vela-{}.tar.gz {} \; && \
		$(DIST_DIRS) zip -r vela-{}.zip {} \; && \
		cd ../kubectl-vela && \
		$(DIST_DIRS) cp ../../LICENSE {} \; && \
		$(DIST_DIRS) cp ../../README.md {} \; && \
		$(DIST_DIRS) tar -zcf kubectl-vela-{}.tar.gz {} \; && \
		$(DIST_DIRS) zip -r kubectl-vela-{}.zip {} \; && \
		cd .. && \
		sha256sum vela/vela-* kubectl-vela/kubectl-vela-* > sha256sums.txt \
	)

# Run against the configured Kubernetes cluster in ~/.kube/config
run:
	go run ./cmd/core/main.go --application-revision-limit 5

run-apiserver:
	go run ./cmd/apiserver/main.go

# Run go fmt against code
fmt: goimports installcue
	go fmt ./...
	$(GOIMPORTS) -local github.com/oam-dev/kubevela -w $$(go list -f {{.Dir}} ./...)
	$(CUE) fmt ./vela-templates/definitions/internal/*
	$(CUE) fmt ./vela-templates/definitions/registry/*
	$(CUE) fmt ./pkg/stdlib/pkgs/*
	$(CUE) fmt ./pkg/stdlib/op.cue
	$(CUE) fmt ./pkg/workflow/tasks/template/static/*
# Run go vet against code
vet:
	go vet ./...

staticcheck: staticchecktool
	$(STATICCHECK) ./...

lint: golangci
	$(GOLANGCILINT) run ./...

reviewable: manifests fmt vet lint staticcheck
	go mod tidy

# Execute auto-gen code commands and ensure branch is clean.
check-diff: reviewable
	git --no-pager diff
	git diff --quiet || ($(ERR) please run 'make reviewable' to include all changes && false)
	@$(OK) branch is clean

# Build the docker image
docker-build: docker-build-core docker-build-apiserver
	@$(OK)
docker-build-core:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_CORE_IMAGE) .
docker-build-apiserver:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_APISERVER_IMAGE) -f Dockerfile.apiserver .

# Build the runtime docker image
docker-build-runtime-rollout:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_RUNTIME_ROLLOUT_IMAGE) -f runtime/rollout/Dockerfile .

# Push the docker image
docker-push:
	docker push $(VELA_CORE_IMAGE)

e2e-setup-core:
	sh ./hack/e2e/modify_charts.sh
	helm upgrade --install --create-namespace --namespace vela-system --set image.pullPolicy=IfNotPresent --set image.repository=vela-core-test --set applicationRevisionLimit=5 --set dependCheckWait=10s --set image.tag=$(GIT_COMMIT) --set multicluster.enabled=true --wait kubevela ./charts/vela-core
	kubectl wait --for=condition=Available deployment/kubevela-vela-core -n vela-system --timeout=180s

setup-runtime-e2e-cluster:
	helm upgrade --install --create-namespace --namespace vela-system --kubeconfig=$(RUNTIME_CLUSTER_CONFIG) --set image.pullPolicy=IfNotPresent --set image.repository=vela-runtime-rollout-test --set image.tag=$(GIT_COMMIT) --wait vela-rollout ./runtime/rollout/charts

e2e-setup:
	helm install kruise https://github.com/openkruise/kruise/releases/download/v0.9.0/kruise-chart.tgz --set featureGates="PreDownloadImageForInPlaceUpdate=true"
	sh ./hack/e2e/modify_charts.sh
	helm upgrade --install --create-namespace --namespace vela-system --set image.pullPolicy=IfNotPresent --set image.repository=vela-core-test --set applicationRevisionLimit=5 --set dependCheckWait=10s --set image.tag=$(GIT_COMMIT) --wait kubevela ./charts/vela-core
	helm upgrade --install --create-namespace --namespace oam-runtime-system --set image.pullPolicy=IfNotPresent --set image.repository=vela-core-test --set dependCheckWait=10s --set image.tag=$(GIT_COMMIT) --wait oam-runtime ./charts/oam-runtime
	bin/vela addon enable fluxcd
	bin/vela addon enable terraform
	ginkgo version
	ginkgo -v -r e2e/setup

	timeout 600s bash -c -- 'while true; do kubectl get ns flux-system; if [ $$? -eq 0 ] ; then break; else sleep 5; fi;done'
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=vela-core,app.kubernetes.io/instance=kubevela -n vela-system --timeout=600s
	kubectl wait --for=condition=Ready pod -l app=source-controller -n flux-system --timeout=600s
	kubectl wait --for=condition=Ready pod -l app=helm-controller -n flux-system --timeout=600s

build-swagger:
	go run ./cmd/apiserver/main.go build-swagger ./docs/apidoc/swagger.json
e2e-api-test:
	# Run e2e test
	ginkgo -v -skipPackage capability,setup,application -r e2e
	ginkgo -v -r e2e/application

e2e-apiserver-test: build-swagger
	go test -v -coverpkg=./... -coverprofile=/tmp/e2e_apiserver_test.out ./test/e2e-apiserver-test
	@$(OK) tests pass

e2e-test:
	# Run e2e test
	ginkgo -v  --skip="rollout related e2e-test." ./test/e2e-test
	@$(OK) tests pass

e2e-addon-test:
	cp bin/vela /tmp/
	ginkgo -v ./test/e2e-addon-test
	@$(OK) tests pass

e2e-rollout-test:
	ginkgo -v  --focus="rollout related e2e-test." ./test/e2e-test
	@$(OK) tests pass

e2e-multicluster-test:
	go test -v -coverpkg=./... -coverprofile=/tmp/e2e_multicluster_test.out ./test/e2e-multicluster-test
	@$(OK) tests pass

compatibility-test: vet lint staticcheck generate-compatibility-testdata
	# Run compatibility test with old crd
	COMPATIBILITY_TEST=TRUE go test -race $(shell go list ./pkg/...  | grep -v apiserver)
	@$(OK) compatibility-test pass

generate-compatibility-testdata:
	mkdir -p  ./test/compatibility-test/testdata
	go run ./test/compatibility-test/convert/main.go ./charts/vela-core/crds ./test/compatibility-test/testdata

compatibility-testdata-cleanup:
	rm -f ./test/compatibility-test/testdata/*

e2e-cleanup:
	# Clean up
	rm -rf ~/.vela

image-cleanup:
# Delete Docker image
ifneq ($(shell docker images -q $(VELA_CORE_TEST_IMAGE)),)
	docker rmi -f $(VELA_CORE_TEST_IMAGE)
endif

end-e2e-core:
	sh ./hack/e2e/end_e2e_core.sh

end-e2e:
	sh ./hack/e2e/end_e2e.sh

# load docker image to the kind cluster
kind-load:
	docker build -t $(VELA_CORE_TEST_IMAGE) -f Dockerfile.e2e .
	kind load docker-image $(VELA_CORE_TEST_IMAGE) || { echo >&2 "kind not installed or error loading image: $(VELA_CORE_TEST_IMAGE)"; exit 1; }

kind-load-runtime-cluster:
	/bin/sh hack/e2e/build_runtime_rollout.sh
	docker build -t $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE) -f runtime/rollout/e2e/Dockerfile.e2e runtime/rollout/e2e/
	rm -rf runtime/rollout/e2e/tmp
	kind load docker-image $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE) --name=$(RUNTIME_CLUSTER_NAME)  || { echo >&2 "kind not installed or error loading image: $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE)"; exit 1; }

# Run tests
core-test: fmt vet manifests
	go test ./pkg/... -coverprofile cover.out

# Build vela core manager and apiserver binary
manager: fmt vet lint manifests
	$(GOBUILD_ENV) go build -o bin/manager -a -ldflags $(LDFLAGS) ./cmd/core/main.go
	$(GOBUILD_ENV) go build -o bin/apiserver -a -ldflags $(LDFLAGS) ./cmd/apiserver/main.go

vela-runtime-rollout-manager: fmt vet lint manifests
	$(GOBUILD_ENV) go build -o ./runtime/rollout/bin/manager -a -ldflags $(LDFLAGS) ./runtime/rollout/cmd/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
core-run: fmt vet manifests
	go run ./cmd/core/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config with debug logs
core-debug-run: fmt vet manifests
	go run ./cmd/core/main.go --log-debug=true

# Install CRDs and Definitions of Vela Core into a cluster, this is for develop convenient.
core-install: manifests
	kubectl apply -f hack/namespace.yaml
	kubectl apply -f charts/vela-core/crds/
	@$(OK) install succeed

# Uninstall CRDs and Definitions of Vela Core from a cluster, this is for develop convenient.
core-uninstall: manifests
	kubectl delete -f charts/vela-core/crds/

# Generate manifests e.g. CRD, RBAC etc.
manifests: installcue kustomize addon
	go generate $(foreach t,pkg apis,./$(t)/...)
	# TODO(yangsoon): kustomize will merge all CRD into a whole file, it may not work if we want patch more than one CRD in this way
	$(KUSTOMIZE) build config/crd -o config/crd/base/core.oam.dev_applications.yaml
	./hack/crd/cleanup.sh
	go run ./hack/crd/dispatch/dispatch.go config/crd/base charts/vela-core/crds charts/oam-runtime/crds runtime/ charts/vela-minimal/crds
	rm -f config/crd/base/*
	./vela-templates/gen_definitions.sh

GOLANGCILINT_VERSION ?= v1.38.0
HOSTOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
HOSTARCH := $(shell uname -m)
ifeq ($(HOSTARCH),x86_64)
HOSTARCH := amd64
endif

golangci:
ifneq ($(shell which golangci-lint),)
	@$(OK) golangci-lint is already installed
GOLANGCILINT=$(shell which golangci-lint)
else ifeq (, $(shell which $(GOBIN)/golangci-lint))
	@{ \
	set -e ;\
	echo 'installing golangci-lint-$(GOLANGCILINT_VERSION)' ;\
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) $(GOLANGCILINT_VERSION) ;\
	echo 'Successfully installed' ;\
	}
GOLANGCILINT=$(GOBIN)/golangci-lint
else
	@$(OK) golangci-lint is already installed
GOLANGCILINT=$(GOBIN)/golangci-lint
endif

.PHONY: staticchecktool
staticchecktool:
ifeq (, $(shell which staticcheck))
	@{ \
	set -e ;\
	echo 'installing honnef.co/go/tools/cmd/staticcheck ' ;\
	GO111MODULE=off go get honnef.co/go/tools/cmd/staticcheck ;\
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
	GO111MODULE=off go get -u golang.org/x/tools/cmd/goimports ;\
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
	GO111MODULE=off go get -u cuelang.org/go/cmd/cue ;\
	}
CUE=$(GOBIN)/cue
else
CUE=$(shell which cue)
endif

KUSTOMIZE_VERSION ?= 3.8.2

.PHONY: kustomize
kustomize:
ifeq (, $(shell kustomize version | grep $(KUSTOMIZE_VERSION)))
	@{ \
	set -eo pipefail ;\
	echo 'installing kustomize-v$(KUSTOMIZE_VERSION) into $(GOBIN)' ;\
	curl -sS https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh | bash -s $(KUSTOMIZE_VERSION) $(GOBIN);\
	echo 'Install succeed' ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

check-license-header:
	./hack/licence/header-check.sh

def-install:
	./hack/utils/installdefinition.sh

# generate addons to auto-gen and charts
addon:
	go run ./vela-templates/gen_addons.go
