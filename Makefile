# Vela version
VELA_VERSION ?= master
# Repo info
GIT_COMMIT          ?= git-$(shell git rev-parse --short HEAD)
GIT_COMMIT_LONG     ?= $(shell git rev-parse HEAD)
VELA_VERSION_VAR    := github.com/oam-dev/kubevela/version.VelaVersion
VELA_GITVERSION_VAR := github.com/oam-dev/kubevela/version.GitRevision
LDFLAGS             ?= "-X $(VELA_VERSION_VAR)=$(VELA_VERSION) -X $(VELA_GITVERSION_VAR)=$(GIT_COMMIT)"

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

all: build

# Run tests
test: vet lint staticcheck
	go test -race -coverprofile=coverage.txt -covermode=atomic ./pkg/... ./cmd/...
	go test -race -covermode=atomic ./references/apiserver/... ./references/cli/... ./references/common/...
	@$(OK) unit-tests pass

# Build manager binary
build: fmt vet lint staticcheck
	go run hack/chart/generate.go
	go build -o bin/vela -ldflags ${LDFLAGS} references/cmd/cli/main.go
	git checkout references/cmd/cli/fake/chart_source.go
	@$(OK) build succeed

vela-cli:
	go run hack/chart/generate.go
	go build -o bin/vela -ldflags ${LDFLAGS} references/cmd/cli/main.go
	git checkout references/cmd/cli/fake/chart_source.go

dashboard-build:
	cd dashboard && npm install && cd ..

doc-gen:
	rm -r docs/en/cli/*
	go run hack/docgen/gen.go
	go run hack/references/generate.go

api-gen:
	swag init -g references/apiserver/route.go --output references/apiserver/docs
	swagger-codegen generate -l html2 -i references/apiserver/docs/swagger.yaml -o references/apiserver/docs
	mv references/apiserver/docs/index.html docs/en/developers/references/restful-api/

generate-source:
	go run hack/frontend/source.go

cross-build:
	go run hack/chart/generate.go
	GO111MODULE=on CGO_ENABLED=0 $(GOX) -ldflags $(LDFLAGS) -parallel=2 -output="_bin/{{.OS}}-{{.Arch}}/vela" -osarch='$(TARGETS)' ./references/cmd/cli/

compress:
	( \
		echo "\n## Release Info\nVERSION: $(VELA_VERSION)" >> README.md && \
		echo "GIT_COMMIT: $(GIT_COMMIT_LONG)\n" >> README.md && \
		cd _bin && \
		$(DIST_DIRS) cp ../LICENSE {} \; && \
		$(DIST_DIRS) cp ../README.md {} \; && \
		$(DIST_DIRS) tar -zcf vela-{}.tar.gz {} \; && \
		$(DIST_DIRS) zip -r vela-{}.zip {} \; && \
		sha256sum vela-* > sha256sums.txt \
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
	$(GOLANGCILINT) run  ./...

reviewable: manifests fmt vet lint staticcheck
	go mod tidy

# Execute auto-gen code commands and ensure branch is clean.
check-diff: reviewable
	git diff --quiet || ($(ERR) please run 'make reviewable' to include all changes && false)
	@$(OK) branch is clean

# Build the docker image
docker-build:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

e2e-setup:
	helm install --create-namespace -n flux-system helm-flux http://oam.dev/catalog/helm-flux2-0.1.0.tgz
	helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz
	helm repo add jetstack https://charts.jetstack.io
	helm repo update
	helm upgrade --install --create-namespace --namespace cert-manager cert-manager jetstack/cert-manager --version v1.2.0  --set installCRDs=true --wait
	helm upgrade --install --create-namespace --namespace vela-system --set image.pullPolicy=IfNotPresent --set admissionWebhooks.certManager.enabled=true --set image.repository=vela-core-test --set image.tag=$(GIT_COMMIT) --wait kubevela ./charts/vela-core
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

e2e-rollout-test:
	ginkgo -v --focus="Cloneset based rollout tests" ./test/e2e-test/

e2e-test:
	# Run e2e test
	ginkgo -v --skip="Cloneset based rollout tests" ./test/e2e-test
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
ifneq ($(shell docker images -q vela-core-test:$(GIT_COMMIT)),)
	docker image rm -f vela-core-test:$(GIT_COMMIT)
endif

# load docker image to the kind cluster
kind-load:
	docker build -t vela-core-test:$(GIT_COMMIT) .
	kind load docker-image vela-core-test:$(GIT_COMMIT) || { echo >&2 "kind not installed or error loading image: $(IMAGE)"; exit 1; }

# Image URL to use all building/pushing image targets
IMG ?= vela-core:latest

# Run tests
core-test: fmt vet manifests
	go test ./pkg/... -coverprofile cover.out

# Build manager binary
manager: fmt vet lint manifests
	go build -o bin/manager ./cmd/core/main.go

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
manifests:
	go generate $(foreach t,pkg apis,./$(t)/...)
	./hack/vela-templates/gen_definitions.sh

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

start-dashboard:
	go run references/cmd/apiserver/main.go &
	cd references/dashboard && npm install && npm start && cd ..

swagger-gen:
	$(GOBIN)/swag init -g apiserver/route.go -d pkg/ -o references/apiserver/docs/
