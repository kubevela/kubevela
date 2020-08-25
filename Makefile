# Vela version
VELA_VERSION ?= 0.1.0
# Repo info
GIT_COMMIT ?= git-$(shell git rev-parse --short HEAD)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: build

# Run tests
test: fmt vet
	go test ./pkg/... -coverprofile cover.out

# Build manager binary
build: fmt vet
	go build -ldflags "-X github.com/cloud-native-application/rudrx/version.VelaVersion=${VELA_VERSION} -X github.com/cloud-native-application/rudrx/version.GitRevision=${GIT_COMMIT}" -o bin/vela cmd/vela/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: fmt vet
	go run ./cmd/server/main.go

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

e2e-setup:
	ginkgo version
	ginkgo -v -r e2e/setup
	bin/vela dashboard &

e2e-test:
	# Run e2e test
	ginkgo -v -skipPackage setup,apiserver -r e2e

e2e-api-test:
	# Run e2e test
	ginkgo -v -r e2e/apiserver

e2e-cleanup:
	# Clean up