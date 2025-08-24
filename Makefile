include makefiles/const.mk
include makefiles/dependency.mk
include makefiles/release.mk
include makefiles/develop.mk
include makefiles/build.mk
# Install kubebuilder for testing
.PHONY: setup-kubebuilder
setup-kubebuilder:
	./hack/setup-kubebuilder.sh

# Run tests with kubebuilder setup
.PHONY: test-with-kubebuilder
test-with-kubebuilder: setup-kubebuilder
	KUBEBUILDER_ASSETS=/usr/local/kubebuilder/bin go test ./... -v
.PHONY: vendor-patched-deps
vendor-patched-deps:
	@echo "===========> Vendoring patched dependencies"
	@mkdir -p vendor/github.com/rubenv/sql-migrate
	@cp -r $(shell go env GOPATH)/pkg/mod/github.com/rubenv/sql-migrate@v1.5.2/* vendor/github.com/rubenv/sql-migrate/
	@echo "===========> Patched dependencies have been vendored"
include makefiles/e2e.mk

.DEFAULT_GOAL := all
all: build

## e2e-debug: Debug e2e test environment
e2e-debug: vela-cli
	@echo "===========> Debugging e2e test environment"
	@echo "Current directory: $(shell pwd)"
	@echo "Checking vela binary:"
	@ls -la $(shell pwd)/bin/vela || echo "Binary not found!"
	@if [ -f "$(shell pwd)/bin/vela" ]; then \
		echo "Binary exists, checking permissions:"; \
		stat $(shell pwd)/bin/vela; \
		echo "Testing binary execution:"; \
		$(shell pwd)/bin/vela version 2>&1 || echo "Execution failed"; \
	fi
	@echo "===========> Environment PATH: $$PATH"
	@echo "===========> Debug information complete"

# ==============================================================================
# Targets

## setup-kubebuilder: Install kubebuilder binaries for testing
setup-kubebuilder:
	@echo "===========> Setting up kubebuilder"
	@mkdir -p /usr/local/kubebuilder/bin || sudo mkdir -p /usr/local/kubebuilder/bin
	@if [ ! -w /usr/local/kubebuilder/bin ]; then \
		echo "===========> Need sudo to write to /usr/local/kubebuilder/bin"; \
		sudo chmod -R 777 /usr/local/kubebuilder/bin || true; \
	fi
	@echo "===========> Downloading kubebuilder"
	@curl -L --retry 5 --retry-delay 3 https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.9.0/kubebuilder_linux_amd64 -o /usr/local/kubebuilder/bin/kubebuilder || \
		sudo curl -L --retry 5 --retry-delay 3 https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.9.0/kubebuilder_linux_amd64 -o /usr/local/kubebuilder/bin/kubebuilder
	@chmod +x /usr/local/kubebuilder/bin/kubebuilder || sudo chmod +x /usr/local/kubebuilder/bin/kubebuilder
	@echo "===========> Downloading etcd"
	@mkdir -p /tmp/etcd-download
	@curl -L --retry 5 --retry-delay 3 https://github.com/etcd-io/etcd/releases/download/v3.4.13/etcd-v3.4.13-linux-amd64.tar.gz -o /tmp/etcd.tar.gz
	@tar -xzf /tmp/etcd.tar.gz -C /tmp/etcd-download --strip-components=1
	@cp /tmp/etcd-download/etcd /usr/local/kubebuilder/bin/ || sudo cp /tmp/etcd-download/etcd /usr/local/kubebuilder/bin/
	@cp /tmp/etcd-download/etcdctl /usr/local/kubebuilder/bin/ || sudo cp /tmp/etcd-download/etcdctl /usr/local/kubebuilder/bin/
	@chmod +x /usr/local/kubebuilder/bin/etcd || sudo chmod +x /usr/local/kubebuilder/bin/etcd
	@chmod +x /usr/local/kubebuilder/bin/etcdctl || sudo chmod +x /usr/local/kubebuilder/bin/etcdctl
	@rm -rf /tmp/etcd.tar.gz /tmp/etcd-download
	@echo "===========> Verifying installation"
	@ls -la /usr/local/kubebuilder/bin
	@echo "===========> Kubebuilder setup complete"

## setup-test-env: Set up test environment prerequisites
setup-test-env: envtest setup-kubebuilder
	@$(OK) test environment ready

## test-env-check: Verify test environment is properly set up
test-env-check:
	@echo "===========> Checking test environment"
	@bash hack/test-env.sh
	@echo "===========> Test environment check complete"

## test: Run tests
test: setup-test-env test-env-check unit-test-core test-cli-gen
	@$(OK) unit-tests pass
	
	## e2e-debug: Debug e2e test environment
	e2e-debug: vela-cli
	@echo "===========> Debugging e2e test environment"
	@echo "Current directory: $(shell pwd)"
	@echo "Checking vela binary:"
	@ls -la $(shell pwd)/bin/vela || echo "Binary not found!"
	@if [ -f "$(shell pwd)/bin/vela" ]; then \
		echo "Binary exists, checking permissions:"; \
		stat $(shell pwd)/bin/vela; \
		echo "Testing binary execution:"; \
		$(shell pwd)/bin/vela version 2>&1 || echo "Execution failed"; \
	fi
	@echo "===========> Environment PATH: $$PATH"
	@echo "===========> Debug information complete"

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
##@ Development

.PHONY: setup-kubebuilder
setup-kubebuilder:
	@echo "==> Installing kubebuilder"
	@chmod +x hack/install-kubebuilder.sh
	@hack/install-kubebuilder.sh

.PHONY: setup-test-binaries
setup-test-binaries: ## Download and install binaries needed for running tests
	@echo "Installing test binaries..."
	bash hack/download-binaries.sh
## vet: Run go vet against code
vet:
	@$(INFO) go vet
	@go vet $(shell go list ./...|grep -v scaffold)

## staticcheck: Run the staticcheck
staticcheck: staticchecktool
	@$(INFO) staticcheck
	@$(STATICCHECK) $(shell go list ./...|grep -v scaffold)
# Include dependency makefiles
include makefiles/dependency.mk
include makefiles/develop.mk

# Define standard colors
BLACK        := $(shell tput -Txterm setaf 0)
RED          := $(shell tput -Txterm setaf 1)
GREEN        := $(shell tput -Txterm setaf 2)
YELLOW       := $(shell tput -Txterm setaf 3)
BLUE         := $(shell tput -Txterm setaf 4)
MAGENTA      := $(shell tput -Txterm setaf 5)
CYAN         := $(shell tput -Txterm setaf 6)
WHITE        := $(shell tput -Txterm setaf 7)
RESET        := $(shell tput -Txterm sgr0)

# Define output functions
INFO := @printf "${BLUE}INFO${RESET}: "
OK := @printf "${GREEN}OK${RESET}: "
ERROR := @printf "${RED}ERROR${RESET}: "

# Test setup targets
.PHONY: setup-kubebuilder
setup-kubebuilder:
	@$(INFO) "Installing kubebuilder test binaries"
	@chmod +x hack/setup-kubebuilder.sh
	@./hack/setup-kubebuilder.sh
	@$(OK) "Kubebuilder test binaries installed"

.PHONY: verify-test-env
verify-test-env:
	@$(INFO) "Verifying test environment"
	@chmod +x hack/test-env.sh
	@./hack/test-env.sh
	@$(OK) "Test environment verified"

.PHONY: test-setup
test-setup: setup-kubebuilder verify-test-env
	@$(INFO) "Test environment setup complete"
	@echo "You can now run your tests with: KUBEBUILDER_ASSETS=/usr/local/kubebuilder/bin go test ./..."

# Test targets
.PHONY: test
test: verify-test-env
	@$(INFO) "Running all tests"
	KUBEBUILDER_ASSETS=/usr/local/kubebuilder/bin go test ./... -v

.PHONY: test-unit
test-unit:
	@$(INFO) "Running unit tests"
	go test $(shell go list ./... | grep -v "/e2e/") -v

# Main build targets (add or modify as needed)
.PHONY: build
build:
	@$(INFO) "Building KubeVela"
	go build -o bin/vela ./cmd/core/main.go
	@$(OK) "Build complete"

.PHONY: vela-cli
vela-cli:
	@$(INFO) "Building Vela CLI"
	go build -o bin/vela ./cmd/cli/main.go
	@$(OK) "CLI build complete"
## lint: Run the golangci-lint
lint: golangci
	@$(INFO) lint
	@$(GOLANGCILINT) run --fix --verbose --exclude-dirs 'scaffold'

## reviewable: Run the reviewable
reviewable: manifests fmt vet lint staticcheck helm-doc-gen sdk_fmt
	go mod tidy

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

.PHONY: setup-kubebuilder
setup-kubebuilder:
	@echo "Setting up kubebuilder..."
	@chmod +x hack/download-kubebuilder.sh
	@hack/download-kubebuilder.sh

## install-kustomize: Download and install kustomize locally
install-kustomize:
	@mkdir -p ./bin
	@echo "===========> Installing kustomize to ./bin"
	@curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- 3.8.0 ./bin/
	@chmod +x ./bin/kustomize
	@echo "===========> kustomize installed to $(shell pwd)/bin/kustomize"
	@echo "===========> Please add to your PATH: export PATH=$(shell pwd)/bin:\$$PATH"

## manifests: Generate manifests e.g. CRD, RBAC etc.
manifests: installcue kustomize
	go generate $(foreach t,pkg apis,./$(t)/...)
	# TODO(yangsoon): kustomize will merge all CRD into a whole file, it may not work if we want patch more than one CRD in this way
	$(KUSTOMIZE) build config/crd -o config/crd/base/core.oam.dev_applications.yaml
	go run ./hack/crd/dispatch/dispatch.go config/crd/base charts/vela-core/crds
	rm -f config/crd/base/*
	./vela-templates/gen_definitions.sh
# Fix vendor directory issues
.PHONY: fix-vendor
fix-vendor:
	@echo "Fixing vendor directory..."
	@bash hack/fix-vendor.sh
# Include dependency makefiles
include makefiles/dependency.mk
include makefiles/develop.mk

# Define standard colors
BLACK        := $(shell tput -Txterm setaf 0)
RED          := $(shell tput -Txterm setaf 1)
GREEN        := $(shell tput -Txterm setaf 2)
YELLOW       := $(shell tput -Txterm setaf 3)
BLUE         := $(shell tput -Txterm setaf 4)
MAGENTA      := $(shell tput -Txterm setaf 5)
CYAN         := $(shell tput -Txterm setaf 6)
WHITE        := $(shell tput -Txterm setaf 7)
RESET        := $(shell tput -Txterm sgr0)

# Define output functions
INFO := @printf "${BLUE}INFO${RESET}: "
OK := @printf "${GREEN}OK${RESET}: "
ERROR := @printf "${RED}ERROR${RESET}: "

# Test environment variables
export KUBEBUILDER_DIR ?= /usr/local/kubebuilder
export KUBEBUILDER_BIN ?= $(KUBEBUILDER_DIR)/bin
export KUBEBUILDER_ASSETS ?= $(KUBEBUILDER_BIN)

# Test setup targets
.PHONY: setup-test-env
setup-test-env:
	@$(INFO) "Setting up complete test environment"
	@chmod +x hack/setup-test-env.sh
	@./hack/setup-test-env.sh
	@$(OK) "Test environment setup complete"

.PHONY: verify-test-env
verify-test-env:
	@$(INFO) "Verifying test environment"
	@chmod +x hack/test-env.sh
	@./hack/test-env.sh
	@$(OK) "Test environment verified"

# Test targets
.PHONY: test
test: verify-test-env
	@$(INFO) "Running all tests"
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_BIN) go test ./... -v

.PHONY: test-unit
test-unit:
	@$(INFO) "Running unit tests"
	go test $(shell go list ./... | grep -v "/e2e/") -v

.PHONY: test-with-coverage
test-with-coverage: verify-test-env
	@$(INFO) "Running tests with coverage"
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_BIN) go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@$(OK) "Test coverage report generated in coverage.html"

# Main build targets
.PHONY: build
build:
	@$(INFO) "Building KubeVela"
	go build -o bin/vela ./cmd/core/main.go
	@$(OK) "Build complete"

.PHONY: vela-cli
vela-cli:
	@$(INFO) "Building Vela CLI"
	go build -o bin/vela ./cmd/cli/main.go
	@$(OK) "CLI build complete"

# Help target
.PHONY: help
help:
	@echo "KubeVela Makefile targets:"
	@echo ""
	@echo "Test Environment:"
	@echo "  setup-test-env      - Set up the complete test environment (downloads all required binaries)"
	@echo "  verify-test-env     - Verify that the test environment is properly set up"
	@echo ""
	@echo "Testing:"
	@echo "  test                - Run all tests"
	@echo "  test-unit           - Run unit tests only"
	@echo "  test-with-coverage  - Run tests with coverage report"
	@echo ""
	@echo "Building:"
	@echo "  build               - Build the KubeVela core binary"
	@echo "  vela-cli            - Build the Vela CLI binary"
	@echo ""
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
