

vela-cli:
	$(GOBUILD_ENV) go build -o bin/vela -a -ldflags $(LDFLAGS) ./references/cmd/cli/main.go

kubectl-vela:
	$(GOBUILD_ENV) go build -o bin/kubectl-vela -a -ldflags $(LDFLAGS) ./cmd/plugin/main.go

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
