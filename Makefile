include makefiles/const.mk
include makefiles/dependency.mk
include makefiles/release.mk
include makefiles/develop.mk
include makefiles/build.mk
include makefiles/e2e.mk

.DEFAULT_GOAL := all
all: build

# Run tests
test: unit-test-core test-cli-gen
	@$(OK) unit-tests pass

test-cli-gen: 
	mkdir -p ./bin/doc
	go run ./hack/docgen/cli/gen.go ./bin/doc
unit-test-core:
	go test -coverprofile=coverage.txt $(shell go list ./pkg/... ./cmd/... ./apis/... | grep -v apiserver | grep -v applicationconfiguration)
	go test $(shell go list ./references/... | grep -v apiserver)

# Build vela cli binary
build: vela-cli kubectl-vela
	@$(OK) build succeed

build-cleanup:
	rm -rf _bin

# Run go fmt against code
fmt: goimports installcue
	go fmt ./...
	$(GOIMPORTS) -local github.com/oam-dev/kubevela -w $$(go list -f {{.Dir}} ./...)
	$(CUE) fmt ./vela-templates/definitions/internal/*
	$(CUE) fmt ./vela-templates/definitions/deprecated/*
	$(CUE) fmt ./vela-templates/definitions/registry/*
	$(CUE) fmt ./pkg/stdlib/pkgs/*
	$(CUE) fmt ./pkg/stdlib/op.cue
	$(CUE) fmt ./pkg/workflow/tasks/template/static/*
# Run go vet against code

sdk_fmt:
	./hack/sdk/reviewable.sh

vet:
	@$(INFO) go vet
	@go vet $(shell go list ./...|grep -v scaffold)

staticcheck: staticchecktool
	@$(INFO) staticcheck
	@$(STATICCHECK) $(shell go list ./...|grep -v scaffold)

lint: golangci
	@$(INFO) lint
	@$(GOLANGCILINT) run --skip-dirs 'scaffold'

reviewable: manifests fmt vet lint staticcheck helm-doc-gen sdk_fmt
	go mod tidy

# Execute auto-gen code commands and ensure branch is clean.
check-diff: reviewable
	git --no-pager diff
	git diff --quiet || ($(ERR) please run 'make reviewable' to include all changes && false)
	@$(OK) branch is clean

# Push the docker image
docker-push:
	docker push $(VELA_CORE_IMAGE)



image-cleanup:
ifneq (, $(shell which docker))
# Delete Docker images

ifneq ($(shell docker images -q $(VELA_CORE_TEST_IMAGE)),)
	docker rmi -f $(VELA_CORE_TEST_IMAGE)
endif

ifneq ($(shell docker images -q $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE)),)
	docker rmi -f $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE)
endif

endif



# load docker image to the k3d cluster
image-load:
	docker build -t $(VELA_CORE_TEST_IMAGE) -f Dockerfile.e2e .
	k3d image import $(VELA_CORE_TEST_IMAGE) || { echo >&2 "kind not installed or error loading image: $(VELA_CORE_TEST_IMAGE)"; exit 1; }

image-load-runtime-cluster:
	/bin/sh hack/e2e/build_runtime_rollout.sh
	docker build -t $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE) -f runtime/rollout/e2e/Dockerfile.e2e runtime/rollout/e2e/
	rm -rf runtime/rollout/e2e/tmp
	k3d image import $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE)  || { echo >&2 "kind not installed or error loading image: $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE)"; exit 1; }
	k3d cluster get $(RUNTIME_CLUSTER_NAME) && k3d image import $(VELA_RUNTIME_ROLLOUT_TEST_IMAGE) --cluster=$(RUNTIME_CLUSTER_NAME) || echo "no worker cluster"

# Run tests
core-test:
	go test ./pkg/... -coverprofile cover.out

# Build vela core manager binary
manager:
	$(GOBUILD_ENV) go build -o bin/manager -a -ldflags $(LDFLAGS) ./cmd/core/main.go

vela-runtime-rollout-manager:
	$(GOBUILD_ENV) go build -o ./runtime/rollout/bin/manager -a -ldflags $(LDFLAGS) ./runtime/rollout/cmd/main.go

# Generate manifests e.g. CRD, RBAC etc.
manifests: installcue kustomize
	go generate $(foreach t,pkg apis,./$(t)/...)
	# TODO(yangsoon): kustomize will merge all CRD into a whole file, it may not work if we want patch more than one CRD in this way
	$(KUSTOMIZE) build config/crd -o config/crd/base/core.oam.dev_applications.yaml
	./hack/crd/cleanup.sh
	go run ./hack/crd/dispatch/dispatch.go config/crd/base charts/vela-core/crds runtime/ charts/vela-minimal/crds
	rm -f config/crd/base/*
	./vela-templates/gen_definitions.sh


HOSTOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
HOSTARCH := $(shell uname -m)
ifeq ($(HOSTARCH),x86_64)
HOSTARCH := amd64
endif


check-license-header:
	./hack/licence/header-check.sh

def-install:
	./hack/utils/installdefinition.sh

helm-doc-gen: helmdoc
	readme-generator -v charts/vela-core/values.yaml -r charts/vela-core/README.md
	readme-generator -v charts/vela-minimal/values.yaml -r charts/vela-minimal/README.md