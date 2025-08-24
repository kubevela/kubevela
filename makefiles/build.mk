
.PHONY: vela-cli
vela-cli:
	@echo "===========> Building vela CLI binary"
	@mkdir -p $(shell pwd)/bin
	$(GOBUILD_ENV) go build -o $(shell pwd)/bin/vela -a -ldflags $(LDFLAGS) ./references/cmd/cli/main.go
	@chmod +x $(shell pwd)/bin/vela
	@echo "===========> Built vela CLI binary at $(shell pwd)/bin/vela"
	@ls -la $(shell pwd)/bin/vela || echo "ERROR: Binary not found after build!"

.PHONY: kubectl-vela
kubectl-vela:
	@echo "===========> Building kubectl-vela binary"
	@mkdir -p $(shell pwd)/bin
	$(GOBUILD_ENV) go build -o $(shell pwd)/bin/kubectl-vela -a -ldflags $(LDFLAGS) ./cmd/plugin/main.go
	@chmod +x $(shell pwd)/bin/kubectl-vela
	@echo "===========> Built kubectl-vela binary at $(shell pwd)/bin/kubectl-vela"

# Build the docker image
.PHONY: docker-build
docker-build: docker-build-core docker-build-cli
	@$(OK)

.PHONY: docker-build-core
docker-build-core:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_CORE_IMAGE) .

.PHONY: docker-build-cli
docker-build-cli:
	docker build --build-arg=VERSION=$(VELA_VERSION) --build-arg=GITVERSION=$(GIT_COMMIT) -t $(VELA_CLI_IMAGE)  -f Dockerfile.cli .
