
.PHONY: vela-cli
vela-cli:
	$(GOBUILD_ENV) go build -o bin/vela -a -ldflags $(LDFLAGS) ./references/cmd/cli/main.go

.PHONY: kubectl-vela
kubectl-vela:
	$(GOBUILD_ENV) go build -o bin/kubectl-vela -a -ldflags $(LDFLAGS) ./cmd/plugin/main.go

# Build the docker image
.PHONY: docker-build
docker-build: docker-build-core docker-build-apiserver docker-build-cli
	@$(OK)

.PHONY: docker-build-core
docker-build-core:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_CORE_IMAGE) .

.PHONY: docker-build-cli
docker-build-cli:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_CLI_IMAGE)  -f Dockerfile.cli .

# Build the runtime docker image
.PHONY: docker-build-runtime-rollout
docker-build-runtime-rollout:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_RUNTIME_ROLLOUT_IMAGE) -f runtime/rollout/Dockerfile .
