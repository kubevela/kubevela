
# Run apiserver for velaux(UI)
run-apiserver:
	go run ./cmd/apiserver/main.go

# Install CRDs and Definitions of Vela Core into a cluster, this is for develop convenient.
core-install: manifests
	kubectl apply -f hack/namespace.yaml
	kubectl apply -f charts/vela-core/crds/
	@$(OK) install succeed

# Uninstall CRDs and Definitions of Vela Core from a cluster, this is for develop convenient.
core-uninstall: manifests
	kubectl delete -f charts/vela-core/crds/

# Run against the configured Kubernetes cluster in ~/.kube/config
run:
	go run ./cmd/core/main.go --application-revision-limit 5

# Run against the configured Kubernetes cluster in ~/.kube/config with debug logs
core-debug-run: fmt vet manifests
	go run ./cmd/core/main.go --log-debug=true

# Run against the configured Kubernetes cluster in ~/.kube/config
core-run: fmt vet manifests
	go run ./cmd/core/main.go