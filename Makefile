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

all: build

# Run tests
test: vet lint staticcheck
	go test -race -coverprofile=coverage.txt -covermode=atomic ./pkg/... ./cmd/...
	go test -race -covermode=atomic ./references/apiserver/... ./references/appfile/... ./references/cli/... ./references/common/... ./references/plugins/...
	@$(OK) unit-tests pass

# Build vela cli binary
build: fmt vet lint staticcheck vela-cli kubectl-vela
	@$(OK) build succeed

vela-cli:
	go run hack/chart/generate.go
	$(GOBUILD_ENV) go build -o bin/vela -a -ldflags $(LDFLAGS) ./references/cmd/cli/main.go
	git checkout references/cmd/cli/fake/chart_source.go

kubectl-vela:
	$(GOBUILD_ENV) go build -o bin/kubectl-vela -a -ldflags $(LDFLAGS) ./cmd/plugin/main.go

dashboard-build:
	cd references/dashboard && npm install && cd ..

doc-gen:
	rm -r docs/en/cli/*
	go run hack/docgen/gen.go
	go run hack/references/generate.go

docs-build:
ifneq ($(wildcard git-page),)
	rm -rf git-page
endif
	sh ./hack/website/test-build.sh

docs-start:
ifeq ($(wildcard git-page),)
	git clone --single-branch --depth 1 https://github.com/oam-dev/kubevela.io.git git-page
endif
	rm -r git-page/docs
	rm git-page/sidebars.js
	cat docs/sidebars.js > git-page/sidebars.js
	cp -R docs/en git-page/docs
	cd git-page && yarn install && yarn start

api-gen:
	swag init -g references/apiserver/route.go --output references/apiserver/docs
	swagger-codegen generate -l html2 -i references/apiserver/docs/swagger.yaml -o references/apiserver/docs
	mv references/apiserver/docs/index.html docs/en/developers/references/restful-api/

generate-source:
	go run hack/frontend/source.go

cross-build:
	rm -rf _bin
    go get github.com/mitchellh/gox@v0.4.0
	go run hack/chart/generate.go
	$(GOBUILD_ENV) $(GOX) -ldflags $(LDFLAGS) -parallel=2 -output="_bin/vela/{{.OS}}-{{.Arch}}/vela" -osarch='$(TARGETS)' ./references/cmd/cli
	$(GOBUILD_ENV) $(GOX) -ldflags $(LDFLAGS) -parallel=2 -output="_bin/kubectl-vela/{{.OS}}-{{.Arch}}/kubectl-vela" -osarch='$(TARGETS)' ./cmd/plugin
	git checkout references/cmd/cli/fake/chart_source.go

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
run: fmt vet
	go run ./cmd/core/main.go

# Run go fmt against code
fmt: goimports installcue
	go fmt ./...
	$(GOIMPORTS) -local github.com/oam-dev/kubevela -w ./pkg ./cmd
	$(CUE) fmt ./hack/vela-templates/cue/*

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
docker-build:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_CORE_IMAGE) .

# Push the docker image
docker-push:
	docker push $(VELA_CORE_IMAGE)

e2e-setup:
	helm install --create-namespace -n flux-system helm-flux http://oam.dev/catalog/helm-flux2-0.1.0.tgz
	helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz
	helm upgrade --install --create-namespace --namespace vela-system --set image.pullPolicy=IfNotPresent --set image.repository=vela-core-test --set applicationRevisionLimit=5 --set image.tag=$(GIT_COMMIT) --wait kubevela ./charts/vela-core
	ginkgo version
	ginkgo -v -r e2e/setup
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=vela-core,app.kubernetes.io/instance=kubevela -n vela-system --timeout=600s
	kubectl wait --for=condition=Ready pod -l app=source-controller -n flux-system --timeout=600s
	kubectl wait --for=condition=Ready pod -l app=helm-controller -n flux-system --timeout=600s
	bin/vela dashboard &

e2e-api-test:
	# Run e2e test
	ginkgo -v -skipPackage capability,setup,apiserver,application -r e2e
	ginkgo -v -r e2e/apiserver
	ginkgo -v -r e2e/application

e2e-test:
	# Run e2e test
	ginkgo -v ./test/e2e-test
	@$(OK) tests pass

compatibility-test: vet lint staticcheck generate-compatibility-testdata
	# Run compatibility test with old crd
	COMPATIBILITY_TEST=TRUE go test -race ./pkg/...
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

# load docker image to the kind cluster
kind-load:
	docker build -t $(VELA_CORE_TEST_IMAGE) .
	kind load docker-image $(VELA_CORE_TEST_IMAGE) || { echo >&2 "kind not installed or error loading image: $(VELA_CORE_TEST_IMAGE)"; exit 1; }

# Run tests
core-test: fmt vet manifests
	go test ./pkg/... -coverprofile cover.out

# Build vela core manager binary
manager: fmt vet lint manifests
	$(GOBUILD_ENV) go build -o bin/manager -a -ldflags $(LDFLAGS) ./cmd/core/main.go

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
manifests: kustomize
	go generate $(foreach t,pkg apis,./$(t)/...)
	# TODO(yangsoon): kustomize will merge all CRD into a whole file, it may not work if we want patch more than one CRD in this way
	$(KUSTOMIZE) build config/crd -o config/crd/base/core.oam.dev_applications.yaml
	mv config/crd/base/* charts/vela-core/crds
	./hack/vela-templates/gen_definitions.sh
	./hack/crd/cleanup.sh

GOLANGCILINT_VERSION ?= v1.31.0
HOSTOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
HOSTARCH := $(shell uname -m)
ifeq ($(HOSTARCH),x86_64)
HOSTARCH := amd64
endif

golangci:
ifeq (, $(shell which golangci-lint))
	@{ \
	set -e ;\
	echo 'installing golangci-lint-$(GOLANGCILINT_VERSION)' ;\
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) $(GOLANGCILINT_VERSION) ;\
	echo 'Install succeed' ;\
	}
GOLANGCILINT=$(GOBIN)/golangci-lint
else
GOLANGCILINT=$(shell which golangci-lint)
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
	set -e ;\
	echo 'installing kustomize-v$(KUSTOMIZE_VERSION) into $(GOBIN)' ;\
	curl -s https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh | bash -s $(KUSTOMIZE_VERSION) $(GOBIN);\
	echo 'Install succeed' ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

start-dashboard:
	go run references/cmd/apiserver/main.go &
	cd references/dashboard && npm install && npm start && cd ..

swagger-gen:
	$(GOBIN)/swag init -g apiserver/route.go -d pkg/ -o references/apiserver/docs/

check-license-header:
	./hack/licence/header-check.sh
