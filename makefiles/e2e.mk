.PHONY: e2e-addon-test
E2E_PROCS ?= auto
E2E_TIMEOUT ?= 1h
E2E_REPORT_DIR ?= _artifacts/e2e
E2E_CORE_LABEL_FILTER ?=
E2E_CORE_AUTH ?= 1
E2E_CORE_REPORT ?= e2e-test
E2E_CORE_PACKAGES ?= ./test/e2e-application-test ./test/e2e-definition-test ./test/e2e-config-test ./test/e2e-helm-test

.PHONY: e2e-vela-cli
e2e-vela-cli:
	$(GOBUILD_ENV) go build -o bin/vela -ldflags $(LDFLAGS) ./references/cmd/cli/main.go

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
		--set featureGates.enableApplicationScopedPolicies=true \
		--set featureGates.enableGlobalPolicies=true \
		--set featureGates.enableAddonComponent=true \
		--set featureGates.enableModuleComponent=true \
		--set featureGates.enableCelExpressions=true \
		--set featureGates.requireCelExpressionOptIn=true \
		--set featureGates.enableDefinitionInheritance=true \
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
	    --set featureGates.enableDefinitionInheritance=true \
	    --set featureGates.enableCelExpressions=true  \
	    --set featureGates.requireCelExpressionOptIn=true \
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

.PHONY: e2e-setup-core-module
e2e-setup-core-module: e2e-setup-core-pre-hook e2e-setup-core-wo-auth
	kubectl wait --for=condition=Available deployment/kubevela-vela-core -n vela-system --timeout=180s

.PHONY: e2e-setup-core-minimal
e2e-setup-core-minimal: e2e-setup-core-module

.PHONY: e2e-setup-core-auth
e2e-setup-core-auth: e2e-setup-core-pre-hook e2e-setup-core-w-auth e2e-setup-core-post-hook

.PHONY: e2e-api-test
e2e-api-test:
	# Run e2e test
	ginkgo -v -skipPackage capability,setup,application -r e2e
	ginkgo -v -r e2e/application


.PHONY: e2e-test
e2e-test:
	# Run e2e test (KUBEVELA_E2E_AUTH=1 enables auth-test registry setup)
	mkdir -p "$(E2E_REPORT_DIR)"
	set -eu; workers=$$(bash hack/e2e/ginkgo_workers.sh "$(E2E_PROCS)"); \
	echo "Ginkgo workers: $$workers"; \
	KUBEVELA_E2E_AUTH=$(E2E_CORE_AUTH) ginkgo -v --procs="$$workers" --timeout=$(E2E_TIMEOUT) --label-filter="$(E2E_CORE_LABEL_FILTER)" --json-report="$(E2E_REPORT_DIR)/$(E2E_CORE_REPORT).json" --junit-report="$(E2E_REPORT_DIR)/$(E2E_CORE_REPORT).xml" $(E2E_CORE_PACKAGES)
	@$(OK) tests pass

# Each execution suite owns its test package; common helpers live in e2e-framework.
# Simultaneous suite runs must use separate clusters and kubeconfigs.
.PHONY: e2e-core-application-test e2e-core-definitions-test e2e-core-config-test e2e-core-helm-test e2e-core-helm-auth-test e2e-core-discovery
e2e-core-application-test:
	$(MAKE) e2e-test E2E_CORE_PACKAGES=./test/e2e-application-test E2E_CORE_AUTH=0 E2E_CORE_REPORT=e2e-core-application-test

e2e-core-definitions-test:
	$(MAKE) e2e-test E2E_CORE_PACKAGES=./test/e2e-definition-test E2E_CORE_AUTH=0 E2E_CORE_REPORT=e2e-core-definitions-test

e2e-core-config-test:
	$(MAKE) e2e-test E2E_CORE_PACKAGES=./test/e2e-config-test E2E_CORE_AUTH=0 E2E_CORE_REPORT=e2e-core-config-test

e2e-core-helm-test:
	$(MAKE) e2e-test E2E_CORE_PACKAGES=./test/e2e-helm-test E2E_CORE_AUTH=1 E2E_CORE_REPORT=e2e-core-helm-test

# Compatibility target for running only authentication cases in the merged suite.
e2e-core-helm-auth-test:
	$(MAKE) e2e-test E2E_CORE_PACKAGES=./test/e2e-helm-test E2E_CORE_AUTH=1 E2E_CORE_LABEL_FILTER=helm-auth E2E_CORE_REPORT=e2e-core-helm-auth-test

e2e-core-discovery:
	bash hack/e2e/verify_core_shards.sh

.PHONY: e2e-workers-test
e2e-workers-test:
	bash hack/e2e/ginkgo_workers_test.sh

.PHONY: e2e-module-test
e2e-module-test:
	# Run the module-as-a-component e2e suite (KUBEVELA_E2E_AUTH=1 enables
	# the zot auth-test registry, used by one credentialed-registry test).
	# Kept as its own package/target so a failure elsewhere in e2e-api-test
	# or e2e-test cannot prevent this from running.
	mkdir -p "$(E2E_REPORT_DIR)"
	set -eu; workers=$$(bash hack/e2e/ginkgo_workers.sh "$(E2E_PROCS)"); \
	echo "Ginkgo workers: $$workers"; \
	KUBEVELA_E2E_AUTH=1 ginkgo -v --procs="$$workers" --timeout=$(E2E_TIMEOUT) --json-report="$(E2E_REPORT_DIR)/e2e-module-test.json" --junit-report="$(E2E_REPORT_DIR)/e2e-module-test.xml" ./test/e2e-module-test
	@$(OK) tests pass

.PHONY: e2e-source-test
e2e-source-test:
	mkdir -p "$(E2E_REPORT_DIR)"
	set -eu; workers=$$(bash hack/e2e/ginkgo_workers.sh "$(E2E_PROCS)"); \
	echo "Ginkgo workers: $$workers"; \
	ginkgo -v --procs="$$workers" --timeout=$(E2E_TIMEOUT) --json-report="$(E2E_REPORT_DIR)/e2e-source-test.json" --junit-report="$(E2E_REPORT_DIR)/e2e-source-test.xml" ./test/e2e-source-test
	@$(OK) tests pass

.PHONY: e2e-addon-component-test
e2e-addon-component-test:
	# Run the addon-as-a-component e2e suite. It brings up its own plain-HTTP
	# ChartMuseum in-cluster and pushes the fixtures under
	# test/e2e-addon-component-test/testdata/addon into it, so it needs no
	# external registry and no credentials.
	#
	# Kept as its own package/target for the same reason as e2e-module-test:
	# a failure elsewhere in e2e-api-test or e2e-test must not stop it running.
	# Requires featureGates.enableAddonComponent=true on the installed chart.
	ginkgo -v ./test/e2e-addon-component-test
	@$(OK) tests pass

# Bring up everything the addon-component suite needs on a local k3d cluster,
# then run it. ADDON_E2E_CLUSTER selects the k3d cluster to use.
.PHONY: e2e-addon-component-test-local
ADDON_E2E_CLUSTER ?= addondemo-cluster
e2e-addon-component-test-local:
	@k3d cluster create $(ADDON_E2E_CLUSTER) --servers 1 || true
	docker build -t vela-core:addon-e2e -f Dockerfile . --build-arg=VERSION=addon-e2e --build-arg=GITVERSION=test
	k3d image import vela-core:addon-e2e -c $(ADDON_E2E_CLUSTER)
	# Pre-load the images the suite's registry and fixtures pull, so a slow or
	# rate-limited pull inside the cluster does not read as a spec timeout.
	@set -e ; for img in \
	  ghcr.io/helm/chartmuseum:v0.16.2 \
	  ealen/echo-server:0.9.2 ; do \
	    docker pull $$img ; \
	    k3d image import $$img -c $(ADDON_E2E_CLUSTER) ; \
	done
	kubectl delete validatingwebhookconfiguration kubevela-vela-core-admission 2>/dev/null || true
	helm upgrade --install kubevela ./charts/vela-core \
		--namespace vela-system --create-namespace \
		--set image.repository=vela-core \
		--set image.tag=addon-e2e \
		--set image.pullPolicy=IfNotPresent \
		--set admissionWebhooks.enabled=true \
		--set featureGates.enableAddonComponent=true \
		--set applicationRevisionLimit=5 \
		--set controllerArgs.reSyncPeriod=1m \
		--wait --timeout 5m
	ginkgo -v ./test/e2e-addon-component-test
	@$(OK) tests pass

.PHONY: e2e-addon-module-test
e2e-addon-module-test:
	# Run the addon-imports-modules e2e suite: "type: addon" components whose
	# modules/_imports.cue pulls "type: module" components. It needs both
	# EnableAddonComponent and EnableModuleComponent on, the admission webhook
	# enabled, reSyncPeriod=1m, bin/vela built from this source, and the
	# images below loaded (registry:2 for modules, chartmuseum for addons;
	# see test/e2e-addon-module-test/testdata/registry.yaml).
	mkdir -p "$(E2E_REPORT_DIR)"
	set -eu; workers=$$(bash hack/e2e/ginkgo_workers.sh "$(E2E_PROCS)"); \
	echo "Ginkgo workers: $$workers"; \
	ginkgo -v --procs="$$workers" --timeout=$(E2E_TIMEOUT) --fail-on-empty --json-report="$(E2E_REPORT_DIR)/e2e-addon-module-test.json" --junit-report="$(E2E_REPORT_DIR)/e2e-addon-module-test.xml" ./test/e2e-addon-module-test
	@$(OK) tests pass

.PHONY: e2e-addon-module-discovery
e2e-addon-module-discovery:
	bash hack/e2e/verify_addon_module_execution.sh

# Bring up (or reuse) the k3d cluster the local e2e targets share, build and
# load the vela-core image, preload the registry images, and install the chart
# with the webhook and both component feature gates on.
#
# Port 30500 is the NodePort of the in-cluster OCI registry
# (test/e2e-module-test/testdata/module/registry.yaml, shared with
# test/e2e-addon-module-test) and 30501 the NodePort of the addon ChartMuseum
# (test/e2e-addon-module-test/testdata/registry.yaml). Mapping them onto the
# host is what lets a test process outside the docker network reach them: on
# a Mac the k3d node IP is not routable from the host, so the addon-module
# suite falls back to http://127.0.0.1:<port> for its own pushes and checks
# while the cluster keeps the node-IP URL. An already-existing cluster keeps
# its old port mappings: run `k3d cluster delete kubevela-debug` first to
# pick this up.
.PHONY: e2e-local-cluster
e2e-local-cluster:
	@k3d cluster create kubevela-debug --servers 1 --agents 1 -p "30500:30500@server:0" -p "30501:30501@server:0" || true
	# Build and load image
	docker build -t vela-core:e2e-test -f Dockerfile . --build-arg=VERSION=e2e-test --build-arg=GITVERSION=test
	k3d image import vela-core:e2e-test -c kubevela-debug
	# Pre-load the registry images the suites deploy (zot/chartmuseum/nginx
	# for test/e2e-helm-test, registry:2 and
	# chartmuseum for the module suites). Each command runs on its own line
	# under `set -e` so a failed pull stops the loop (a `&&` chain would
	# swallow the failure as far as `set -e` is concerned).
	# The images are saved with `--platform` (Docker 28+) and the tarball is
	# imported, instead of `k3d image import <name>`. With Docker's containerd
	# image store (Docker Desktop and Rancher Desktop default) a multi-arch
	# image is stored under its full index, attestation manifests included,
	# while only this platform's content is pulled. `docker save <name>`
	# writes that index and `ctr image import` in the node fails on the
	# missing entries with `ctr: content digest sha256:...: not found`
	# (k3d-io/k3d#1372), and k3d still exits 0. A platform-filtered save
	# writes a single-manifest tarball the node accepts. The filtered save is
	# refused for an image that has no manifest for this platform (the zot
	# image is amd64 only) and by Docker before 28; such an image is a single
	# manifest or comes from the classic store, so a plain save works for it.
	@set -e ; for img in \
	  ghcr.io/project-zot/zot-minimal-linux-amd64:v2.1.1 \
	  ghcr.io/helm/chartmuseum:v0.16.2 \
	  docker.io/library/registry:2 \
	  docker.io/library/nginx:1.27-alpine ; do \
	    docker pull $$img ; \
	    tar=$$(mktemp "$${TMPDIR:-/tmp}/k3d-preload.XXXXXX") ; \
	    docker save --platform linux/$(HOSTARCH) -o $$tar $$img || docker save -o $$tar $$img ; \
	    k3d image import $$tar -c kubevela-debug ; \
	    rm -f $$tar ; \
	done
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
		--set featureGates.enableAddonComponent=true \
		--set featureGates.enableModuleComponent=true \
		--set applicationRevisionLimit=5 \
		--set controllerArgs.reSyncPeriod=1m \
		--wait --timeout 3m
	# The suites drive the vela CLI built from this source.
	$(MAKE) vela-cli

# Run the module e2e suites with k3d and webhook validation. The addon-module
# suite goes first: it runs on a Mac (see e2e-local-cluster), whereas the
# module suite's own "vela module deploy" steps need the k3d node IP to be
# routable from the host, which only a Linux host or CI runner has.
.PHONY: e2e-test-local
e2e-test-local: e2e-local-cluster
	$(MAKE) e2e-addon-module-test
	$(MAKE) e2e-module-test
	@$(OK) tests pass

# Run only the addon-module e2e suite against the local k3d cluster.
.PHONY: e2e-addon-module-test-local
e2e-addon-module-test-local: e2e-local-cluster
	$(MAKE) e2e-addon-module-test
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


e2e-addon-test:
	mkdir -p "$(E2E_REPORT_DIR)"
	set -eu; workers=$$(bash hack/e2e/ginkgo_workers.sh "$(E2E_PROCS)"); \
	echo "Ginkgo workers: $$workers"; \
	ginkgo -v --procs="$$workers" --timeout=$(E2E_TIMEOUT) --json-report="$(E2E_REPORT_DIR)/e2e-addon-test.json" --junit-report="$(E2E_REPORT_DIR)/e2e-addon-test.xml" ./test/e2e-addon-test
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
