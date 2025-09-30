
# Install CRDs and Definitions of Vela Core into a cluster, this is for develop convenient.
.PHONY: core-install
core-install: manifests
	kubectl apply -f hack/namespace.yaml
	kubectl apply -f charts/vela-core/crds/
	@$(OK) install succeed

# Uninstall CRDs and Definitions of Vela Core from a cluster, this is for develop convenient.
.PHONY: core-uninstall
core-uninstall: manifests
	kubectl delete -f charts/vela-core/crds/

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run:
	go run ./cmd/core/main.go --application-revision-limit 5

# Run against the configured Kubernetes cluster in ~/.kube/config with debug logs
.PHONY: core-debug-run
core-debug-run: fmt vet manifests
	go run ./cmd/core/main.go --log-debug=true

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: core-run
core-run: fmt vet manifests
	go run ./cmd/core/main.go

## gen-cue: Generate CUE files from Go files. Variable DIR is the directory of the Go files, FLAGS is the flags for vela def gen-cue command.
.PHONY: gen-cue
gen-cue:
	./hack/cuegen/cuegen.sh $(DIR) $(FLAGS)

# ==============================================================================
# Webhook Debug and Development Targets

K3D_CLUSTER_NAME ?= kubevela-debug
K3D_VERSION ?= v1.31.5

## webhook-help: Show webhook debugging help
.PHONY: webhook-help
webhook-help:
	@echo "=== KubeVela Webhook Debugging Guide ==="
	@echo ""
	@echo "Quick Start (recommended):"
	@echo "  1. make webhook-debug-setup   # Complete setup"
	@echo "  2. Start VS Code debugger (F5) with 'Debug Webhook Validation'"
	@echo ""
	@echo "Individual Commands:"
	@echo "  make k3d-create               # Create k3d cluster"
	@echo "  make k3d-delete               # Delete k3d cluster"
	@echo "  make webhook-setup            # Setup webhook (certs + config)"
	@echo "  make webhook-clean            # Clean up webhook setup"

## k3d-create: Create a k3d cluster for debugging
.PHONY: k3d-create
k3d-create:
	@echo "Creating k3d cluster: $(K3D_CLUSTER_NAME)"
	@if k3d cluster list | grep -q "^$(K3D_CLUSTER_NAME)"; then \
		echo "k3d cluster $(K3D_CLUSTER_NAME) already exists"; \
	else \
		k3d cluster create "$(K3D_CLUSTER_NAME)" \
			--servers 1 \
			--agents 1 \
			--wait || (echo "Failed to create k3d cluster" && exit 1); \
	fi
	@kubectl config use-context "k3d-$(K3D_CLUSTER_NAME)"
	@echo "k3d cluster $(K3D_CLUSTER_NAME) ready"

## k3d-delete: Delete the k3d cluster
.PHONY: k3d-delete
k3d-delete:
	@echo "Deleting k3d cluster: $(K3D_CLUSTER_NAME)"
	@k3d cluster delete "$(K3D_CLUSTER_NAME)" || true

## webhook-setup: Setup webhook certificates and configuration
.PHONY: webhook-setup
webhook-setup:
	@echo "Setting up webhook certificates and configuration..."
	@chmod +x hack/debug-webhook-setup.sh
	@./hack/debug-webhook-setup.sh

## webhook-debug-setup: Complete webhook debug environment setup
.PHONY: webhook-debug-setup
webhook-debug-setup:
	@echo "Setting up complete webhook debug environment..."
	@$(MAKE) k3d-create
	@echo "Waiting for cluster to be ready..."
	@sleep 5
	@kubectl wait --for=condition=Ready nodes --all --timeout=60s || true
	@echo "Installing KubeVela CRDs..."
	@$(MAKE) manifests
	@kubectl apply -f charts/vela-core/crds/ --validate=false
	@echo "Setting up webhook..."
	@$(MAKE) webhook-setup

## webhook-clean: Clean up webhook debug environment
.PHONY: webhook-clean
webhook-clean:
	@echo "Cleaning up webhook debug environment..."
	@rm -rf k8s-webhook-server/
	@kubectl delete secret webhook-server-cert -n vela-system --ignore-not-found
	@kubectl delete validatingwebhookconfiguration kubevela-vela-core-admission --ignore-not-found
	@echo "Webhook debug environment cleaned"
