include makefiles/const.mk
include makefiles/dependency.mk
include makefiles/release.mk
include makefiles/develop.mk
include makefiles/build.mk
include makefiles/e2e.mk

.DEFAULT_GOAL := all
all: build

# ==============================================================================
# Targets

## test: Run tests
test: envtest unit-test-core test-cli-gen
	@$(OK) unit-tests pass

## test-cli-gen: Run the unit tests for cli gen
test-cli-gen: 
	@mkdir -p ./bin/doc
	@go run ./hack/docgen/cli/gen.go ./bin/doc

## unit-test-core: Run the unit tests for core
unit-test-core:
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -coverprofile=coverage.txt $(shell go list ./pkg/... ./cmd/... ./apis/... | grep -v apiserver | grep -v applicationconfiguration)
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $(shell go list ./references/... | grep -v apiserver)

## build: Build vela cli binary
build: vela-cli kubectl-vela
	@$(OK) build succeed

## build-cli: Clean build
build-cleanup:
	@echo "===========> Cleaning all build output"
	@rm -rf _bin

## fmt: Run go fmt against code
fmt: goimports installcue
	go fmt ./...
	$(GOIMPORTS) -local github.com/oam-dev/kubevela -w $$(go list -f {{.Dir}} ./...)
	$(CUE) fmt ./vela-templates/definitions/internal/*
	$(CUE) fmt ./vela-templates/definitions/deprecated/*
	$(CUE) fmt ./vela-templates/definitions/registry/*
	$(CUE) fmt ./pkg/workflow/template/static/*
	$(CUE) fmt ./pkg/workflow/providers/...

## sdk_fmt: Run go fmt against code
sdk_fmt:
	./hack/sdk/reviewable.sh

## vet: Run go vet against code
vet:
	@$(INFO) go vet
	@go vet $(shell go list ./...|grep -v scaffold)

## staticcheck: Run the staticcheck
staticcheck: staticchecktool
	@$(INFO) staticcheck
	@$(STATICCHECK) $(shell go list ./...|grep -v scaffold)

## lint: Run the golangci-lint
lint: golangci
	@$(INFO) lint
	@GOLANGCILINT=$(GOLANGCILINT) ./hack/utils/golangci-lint-wrapper.sh

## reviewable: Run the reviewable
## Run make build to compile vela binary before running this target to ensure all generated definitions are up to date.
reviewable: build manifests fmt vet lint staticcheck helm-doc-gen sdk_fmt

# check-diff: Execute auto-gen code commands and ensure branch is clean.
check-diff: reviewable
	git --no-pager diff
	git diff --quiet || ($(ERR) please run 'make reviewable' to include all changes && false)
	@$(OK) branch is clean

## docker-push: Push the docker image
docker-push:
	@echo "===========> Pushing docker image"
	@docker push $(VELA_CORE_IMAGE)

## image-cleanup: Delete Docker images
image-cleanup:
ifneq (, $(shell which docker))
# Delete Docker images

ifneq ($(shell docker images -q $(VELA_CORE_TEST_IMAGE)),)
	docker rmi -f $(VELA_CORE_TEST_IMAGE)
endif

endif

## image-load: load docker image to the kind cluster
image-load:
	docker build -t $(VELA_CORE_TEST_IMAGE) -f Dockerfile.e2e .
	kind load docker-image $(VELA_CORE_TEST_IMAGE) || { echo >&2 "kind not installed or error loading image: $(VELA_CORE_TEST_IMAGE)"; exit 1; }

## core-test: Run tests
core-test:
	go test ./pkg/... -coverprofile cover.out

## manager: Build vela core manager binary
manager:
	$(GOBUILD_ENV) go build -o bin/manager -a -ldflags $(LDFLAGS) ./cmd/core/main.go

## manifests: Generate manifests e.g. CRD, RBAC etc.
manifests: tidy installcue kustomize sync-crds
	go generate $(foreach t,pkg apis,./$(t)/...)
	# TODO(yangsoon): kustomize will merge all CRD into a whole file, it may not work if we want patch more than one CRD in this way
	$(KUSTOMIZE) build config/crd -o config/crd/base/core.oam.dev_applications.yaml
	go run ./hack/crd/dispatch/dispatch.go config/crd/base charts/vela-core/crds
	rm -f config/crd/base/*
	./vela-templates/gen_definitions.sh


HOSTOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
HOSTARCH := $(shell uname -m)
ifeq ($(HOSTARCH),x86_64)
HOSTARCH := amd64
endif


## check-license-header: Check license header
check-license-header:
	./hack/licence/header-check.sh

## def-gen: Install definitions
def-install:
	./hack/utils/installdefinition.sh

## helm-doc-gen: Generate helm chart README.md
helm-doc-gen: helmdoc
	readme-generator -v charts/vela-core/values.yaml -r charts/vela-core/README.md

## help: Display help information
help: Makefile
	@echo ""
	@echo "Usage:"
	@echo ""
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo ""
	@awk -F ':|##' '/^[^\.%\t][^\t]*:.*##/{printf "  \033[36m%-20s\033[0m %s\n", $$1, $$NF}' $(MAKEFILE_LIST) | sort
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'
