.PHONY: e2e-setup-core-pre-hook
e2e-setup-core-pre-hook:
	sh ./hack/e2e/modify_charts.sh

.PHONY: e2e-setup-core-post-hook
e2e-setup-core-post-hook:
	kubectl wait --for=condition=Available deployment/kubevela-vela-core -n vela-system --timeout=180s
	helm install kruise https://github.com/openkruise/charts/releases/download/kruise-1.1.0/kruise-1.1.0.tgz --set featureGates="PreDownloadImageForInPlaceUpdate=true" --set daemon.socketLocation=/run/k3s/containerd/
	kill -9 $(lsof -it:9098) || true
	go run ./e2e/addon/mock &
	bin/vela addon enable ./e2e/addon/mock/testdata/fluxcd
	bin/vela addon enable ./e2e/addon/mock/testdata/terraform
	# Wait for webhook service endpoints to be ready before enabling addons that require webhook validation
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=vela-core -n vela-system --timeout=180s
	bin/vela addon enable ./e2e/addon/mock/testdata/terraform-alibaba ALICLOUD_ACCESS_KEY=xxx ALICLOUD_SECRET_KEY=yyy ALICLOUD_REGION=cn-beijing

	timeout 600s bash -c -- 'while true; do kubectl get ns flux-system; if [ $$? -eq 0 ] ; then break; else sleep 5; fi;done'
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=vela-core,app.kubernetes.io/instance=kubevela -n vela-system --timeout=600s
	kubectl wait --for=condition=Ready pod -l app=source-controller -n flux-system --timeout=600s
	kubectl wait --for=condition=Ready pod -l app=helm-controller -n flux-system --timeout=600s

.PHONY: e2e-setup-core-wo-auth
e2e-setup-core-wo-auth:
	helm upgrade --install                          \
	    --create-namespace                          \
	    --namespace vela-system                     \
	    --set image.pullPolicy=IfNotPresent         \
	    --set image.repository=vela-core-test       \
		--set applicationRevisionLimit=5            \
		--set controllerArgs.reSyncPeriod=1m		\
	    --set optimize.disableComponentRevision=false        \
	    --set image.tag=$(GIT_COMMIT)               \
		--set multicluster.clusterGateway.image.repository=ghcr.io/oam-dev/cluster-gateway \
		--set admissionWebhooks.patch.image.repository=ghcr.io/oam-dev/kube-webhook-certgen/kube-webhook-certgen \
		--set featureGates.enableCueValidation=true \
		--set featureGates.validateResourcesExist=true \
	    --wait kubevela ./charts/vela-core          \
		--debug

.PHONY: e2e-setup-core-w-auth
e2e-setup-core-w-auth:
	helm upgrade --install                              \
	    --create-namespace                              \
	    --namespace vela-system                         \
	    --set image.pullPolicy=IfNotPresent             \
	    --set image.repository=vela-core-test           \
	    --set applicationRevisionLimit=5                \
	    --set optimize.disableComponentRevision=false   \
	    --set image.tag=$(GIT_COMMIT)                   \
	    --wait kubevela                                 \
	    ./charts/vela-core                              \
	    --set authentication.enabled=true               \
	    --set authentication.withUser=true              \
	    --set authentication.groupPattern='*'           \
	    --set featureGates.zstdResourceTracker=true     \
	    --set featureGates.zstdApplicationRevision=true \
	    --set featureGates.validateComponentWhenSharding=true \
	    --set featureGates.validateResourcesExist=true \
	    --set multicluster.clusterGateway.enabled=true  \
			--set multicluster.clusterGateway.image.repository=ghcr.io/oam-dev/cluster-gateway \
			--set admissionWebhooks.patch.image.repository=ghcr.io/oam-dev/kube-webhook-certgen/kube-webhook-certgen \
	    --set sharding.enabled=true                     \
			--debug
	kubectl get deploy kubevela-vela-core -oyaml -n vela-system | \
		sed 's/schedulable-shards=/shard-id=shard-0/g' | \
		sed 's/instance: kubevela/instance: kubevela-shard/g' | \
		sed 's/shard-id: master/shard-id: shard-0/g' | \
		sed 's/name: kubevela/name: kubevela-shard/g' | \
		kubectl apply -f -
	kubectl wait deployment -n vela-system kubevela-shard-vela-core --for condition=Available=True --timeout=90s


.PHONY: e2e-setup-core
e2e-setup-core: e2e-setup-core-pre-hook e2e-setup-core-wo-auth e2e-setup-core-post-hook

.PHONY: e2e-setup-core-auth
e2e-setup-core-auth: e2e-setup-core-pre-hook e2e-setup-core-w-auth e2e-setup-core-post-hook

.PHONY: e2e-api-test
e2e-api-test:
	# Run e2e test
	ginkgo -v -skipPackage capability,setup,application -r e2e
	ginkgo -v -r e2e/application


.PHONY: e2e-test
e2e-test:
	# Run e2e test
	ginkgo -v ./test/e2e-test
	@$(OK) tests pass

# Run e2e tests with k3d and webhook validation
.PHONY: e2e-test-local
e2e-test-local:
	# Create k3d cluster if needed
	@k3d cluster create kubevela-debug --servers 1 --agents 1 || true
	# Build and load image
	docker build -t vela-core:e2e-test -f Dockerfile . --build-arg=VERSION=e2e-test --build-arg=GITVERSION=test
	k3d image import vela-core:e2e-test -c kubevela-debug
	# Deploy with Helm
	kubectl delete validatingwebhookconfiguration kubevela-vela-core-admission 2>/dev/null || true
	helm upgrade --install kubevela ./charts/vela-core \
		--namespace vela-system --create-namespace \
		--set image.repository=vela-core \
		--set image.tag=e2e-test \
		--set image.pullPolicy=IfNotPresent \
		--set admissionWebhooks.enabled=true \
		--set featureGates.enableCueValidation=true \
		--set featureGates.validateResourcesExist=true \
		--set applicationRevisionLimit=5 \
		--set controllerArgs.reSyncPeriod=1m \
		--wait --timeout 3m
	# Run tests
	ginkgo -v ./test/e2e-test
	@$(OK) tests pass

# Run e2e application tests with k3d and webhook validation
.PHONY: e2e-application-test-local
e2e-application-test-local:
	# Create k3d cluster if needed
	@k3d cluster create kubevela-debug --servers 1 --agents 1 || true
	# Build and load image
	docker build -t vela-core:e2e-test -f Dockerfile . --build-arg=VERSION=e2e-test --build-arg=GITVERSION=test
	k3d image import vela-core:e2e-test -c kubevela-debug
	# Deploy with Helm
	kubectl delete validatingwebhookconfiguration kubevela-vela-core-admission 2>/dev/null || true
	helm upgrade --install kubevela ./charts/vela-core \
		--namespace vela-system --create-namespace \
		--set image.repository=vela-core \
		--set image.tag=e2e-test \
		--set image.pullPolicy=IfNotPresent \
		--set admissionWebhooks.enabled=true \
		--set featureGates.enableCueValidation=true \
		--set featureGates.validateResourcesExist=true \
		--set applicationRevisionLimit=5 \
		--set controllerArgs.reSyncPeriod=1m \
		--wait --timeout 3m
	# Clean up any leftover vela resources from previous test runs
	@vela ls -n default --quiet 2>/dev/null | tail -n +2 | awk '{print $$1}' | xargs -I {} vela delete {} -n default -y 2>/dev/null || true
	@vela env delete env-application 2>/dev/null || true
	# Run application tests
	ginkgo -v -r e2e/application
	@$(OK) tests pass
	@$(MAKE) k3d-delete

# Run main_e2e_test.go with k3d cluster and embedded test binary
.PHONY: e2e-test-main-local
e2e-test-main-local:
	@echo "==> Setting up k3d cluster for main_e2e_test..."
	# Delete existing cluster if it exists and recreate
	@k3d cluster delete kubevela-e2e-main 2>/dev/null || true
	@k3d cluster create kubevela-e2e-main --servers 1 --agents 1
	@echo "==> Building test binary with Dockerfile.e2e..."
	# Detect architecture for proper binary naming
	$(eval ARCH := $(shell uname -m | sed 's/x86_64/amd64/; s/aarch64\|arm64/arm64/'))
	@echo "    Detected architecture: $(ARCH)"
	# Build test image with embedded e2e test
	# Note: Use 'make e2e-test-main-rebuild' if you get "manager-${ARCH}: not found" errors
	docker build -t vela-core:e2e-main-test -f Dockerfile.e2e . \
		--no-cache \
		--build-arg=TARGETARCH=$(ARCH) \
		--build-arg=VERSION=e2e-main-test \
		--build-arg=GITVERSION=test
	# Load image into k3d cluster
	k3d image import vela-core:e2e-main-test -c kubevela-e2e-main
	@echo "==> Modifying Helm charts to enable e2e test..."
	# Backup original chart
	@cp ./charts/vela-core/templates/kubevela-controller.yaml ./charts/vela-core/templates/kubevela-controller.yaml.bak || true
	# Modify charts to add test flags
	sh ./hack/e2e/modify_charts.sh
	@echo "==> Deploying vela-core with embedded test..."
	# Clean up any existing webhook configs
	kubectl delete validatingwebhookconfiguration kubevela-vela-core-admission 2>/dev/null || true
	# Deploy with test binary and flags
	helm upgrade --install kubevela ./charts/vela-core \
		--namespace vela-system --create-namespace \
		--set image.repository=vela-core \
		--set image.tag=e2e-main-test \
		--set image.pullPolicy=IfNotPresent \
		--set admissionWebhooks.enabled=false \
		--set multicluster.enabled=false \
		--set multicluster.clusterGateway.enabled=false \
		--set featureGates.enableCueValidation=true \
		--set featureGates.validateResourcesExist=true \
		--set applicationRevisionLimit=5 \
		--set controllerArgs.reSyncPeriod=1m \
		--wait --timeout 3m
	@echo "==> Waiting for test to complete..."
	# Give the test time to run (it starts the server and runs tests)
	@sleep 10
	@echo "==> Checking test results from pod logs..."
	# Get the pod name and check logs for test results
	@kubectl logs -n vela-system -l app.kubernetes.io/name=vela-core --tail=100 | grep -E "PASS|FAIL|TestE2EMain" || true
	@echo "==> Test coverage will be available at /workspace/data/e2e-profile.out in the pod"
	# Optionally copy coverage data from pod
	@POD=$$(kubectl get pod -n vela-system -l app.kubernetes.io/name=vela-core -o jsonpath='{.items[0].metadata.name}') && \
		kubectl cp vela-system/$$POD:/workspace/data/e2e-profile.out ./e2e-main-coverage.out 2>/dev/null || \
		echo "Coverage data not yet available or test still running"
	# Restore original chart
	@mv ./charts/vela-core/templates/kubevela-controller.yaml.bak ./charts/vela-core/templates/kubevela-controller.yaml 2>/dev/null || true
	@echo "==> Done. Check pod logs for detailed test output:"
	@echo "    kubectl logs -n vela-system -l app.kubernetes.io/name=vela-core -f"
	@$(OK) main_e2e_test setup complete

# Clean up k3d cluster used for main_e2e_test
.PHONY: e2e-test-main-clean
e2e-test-main-clean:
	@echo "==> Cleaning up k3d cluster for main_e2e_test..."
	k3d cluster delete kubevela-e2e-main || true
	# Restore original chart if backup exists
	@mv ./charts/vela-core/templates/kubevela-controller.yaml.bak ./charts/vela-core/templates/kubevela-controller.yaml 2>/dev/null || true
	@echo "==> Cleanup complete"


.PHONY: e2e-addon-test
e2e-addon-test:
	cp bin/vela /tmp/
	ginkgo -v ./test/e2e-addon-test
	@$(OK) tests pass

.PHONY: e2e-multicluster-test
e2e-multicluster-test:
	cd ./test/e2e-multicluster-test && go test -timeout=30m -v -ginkgo.v -ginkgo.trace -coverpkg=./... -coverprofile=/tmp/e2e_multicluster_test.out
	@$(OK) tests pass

.PHONY: e2e-cleanup
e2e-cleanup:
	# Clean up
	rm -rf ~/.vela

.PHONY: end-e2e-core
end-e2e-core:
	sh ./hack/e2e/end_e2e_core.sh

.PHONY: end-e2e-core-shards
end-e2e-core-shards: end-e2e-core
	CORE_NAME=kubevela-shard sh ./hack/e2e/end_e2e_core.sh

.PHONY: end-e2e
end-e2e:
	sh ./hack/e2e/end_e2e.sh
