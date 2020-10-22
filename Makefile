# Vela version
VELA_VERSION ?= 0.1.0
# Repo info
GIT_COMMIT ?= git-$(shell git rev-parse --short HEAD)
VELA_VERSION_VAR := github.com/oam-dev/kubevela/version.VelaVersion
VELA_GITVERSION_VAR := github.com/oam-dev/kubevela/version.GitRevision
LDFLAGS ?= "-X $(VELA_VERSION_VAR)=$(VELA_VERSION) -X $(VELA_GITVERSION_VAR)=$(GIT_COMMIT)"

GOX      = go run github.com/mitchellh/gox
TARGETS  := darwin/amd64 linux/amd64 windows/amd64
DIST_DIRS       := find * -type d -exec

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: build

# Run tests
test: fmt vet lint
	go test -race -coverprofile=coverage.txt -covermode=atomic ./pkg/... ./cmd/...

# Build manager binary
build: fmt vet lint
	go run hack/chart/generate.go
	go build -o bin/vela -ldflags ${LDFLAGS} cmd/vela/main.go
	git checkout cmd/vela/fake/chart_source.go

npm-build:
	cd dashboard && npm run build && cd ./..

npm-install:
	cd dashboard && npm install && cd ./..

generate-doc:
	rm -r documentation/cli/*
	go run hack/docgen/gen.go

generate-source:
	go run hack/frontend/source.go

cross-build:
	go run hack/chart/generate.go
	GO111MODULE=on CGO_ENABLED=0 $(GOX) -ldflags $(LDFLAGS) -parallel=3 -output="_bin/{{.OS}}-{{.Arch}}/vela" -osarch='$(TARGETS)' ./cmd/vela/

compress:
	( \
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
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

lint: golangci
	$(GOLANGCILINT) run --timeout 10m -E golint,goimports  ./...

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

e2e-setup:
	bin/vela install --image-pull-policy IfNotPresent --image-repo vela-core-test --image-tag $(GIT_COMMIT)
	ginkgo version
	ginkgo -v -r e2e/setup
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=vela-core,app.kubernetes.io/instance=kubevela -n vela-system --timeout=600s
	bin/vela dashboard &

e2e-test:
	# Run e2e test
	ginkgo -v -skipPackage setup,apiserver -r e2e

e2e-api-test:
	# Run e2e test
	ginkgo -v -r e2e/apiserver

e2e-cleanup:
	# Clean up

# load docker image to the kind cluster
kind-load:
	docker build -t vela-core-test:$(GIT_COMMIT) .
	kind load docker-image vela-core-test:$(GIT_COMMIT) || { echo >&2 "kind not installed or error loading image: $(IMAGE)"; exit 1; }

# Image URL to use all building/pushing image targets
IMG ?= vela-core:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:crdVersions=v1"

# Run tests
core-test: generate fmt vet manifests
	go test ./pkg/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet lint manifests
	go build -o bin/manager ./cmd/core/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
core-run: generate fmt vet manifests
	go run ./cmd/core/main.go

# Install CRDs and Definitions of Vela Core into a cluster, this is for develop convenient.
core-install: manifests
	kubectl apply -f charts/vela-core/crds/
	kubectl apply -f charts/vela-core/templates/definitions/
	kubectl apply -f charts/third_party/prometheus

# Uninstall CRDs and Definitions of Vela Core from a cluster, this is for develop convenient.
core-uninstall: manifests
	kubectl delete -f charts/vela-core/crds/
	kubectl delete -f charts/vela-core/templates/definitions/
	kubectl delete -f charts/third_party/prometheus

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=charts/vela-core/crds
	rm charts/vela-core/crds/_.yaml

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

GOLANGCILINT_VERSION ?= v1.29.0
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
