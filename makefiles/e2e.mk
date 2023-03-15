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
	bin/vela addon enable ./e2e/addon/mock/testdata/rollout
	bin/vela addon enable ./e2e/addon/mock/testdata/terraform
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
	    --set optimize.disableComponentRevision=false        \
	    --set dependCheckWait=10s                   \
	    --set image.tag=$(GIT_COMMIT)               \
	    --wait kubevela ./charts/vela-core

.PHONY: e2e-setup-core-w-auth
e2e-setup-core-w-auth:
	helm upgrade --install                              \
	    --create-namespace                              \
	    --namespace vela-system                         \
	    --set image.pullPolicy=IfNotPresent             \
	    --set image.repository=vela-core-test           \
	    --set applicationRevisionLimit=5                \
	    --set optimize.disableComponentRevision=false   \
	    --set dependCheckWait=10s                       \
	    --set image.tag=$(GIT_COMMIT)                   \
	    --wait kubevela                                 \
	    ./charts/vela-core                              \
	    --set authentication.enabled=true               \
	    --set authentication.withUser=true              \
	    --set authentication.groupPattern=*             \
	    --set featureGates.zstdResourceTracker=true     \
	    --set featureGates.zstdApplicationRevision=true \
	    --set featureGates.validateComponentWhenSharding=true \
	    --set multicluster.clusterGateway.enabled=true \
	    --set sharding.enabled=true
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

.PHONY: setup-runtime-e2e-cluster
setup-runtime-e2e-cluster:
	helm upgrade --install                               \
	    --namespace vela-system                          \
	    --wait oam-rollout                               \
	    --set image.repository=vela-runtime-rollout-test \
	    --set image.tag=$(GIT_COMMIT)                    \
	    --set applicationRevisionLimit=6                 \
	    --set optimize.disableComponentRevision=false             \
	    ./runtime/rollout/charts

	k3d cluster get $(RUNTIME_CLUSTER_NAME) && 			 \
	helm upgrade --install                               \
	    --create-namespace                               \
	    --namespace vela-system                          \
	    --kubeconfig=$(RUNTIME_CLUSTER_CONFIG)           \
	    --set image.pullPolicy=IfNotPresent              \
	    --set image.repository=vela-runtime-rollout-test \
	    --set image.tag=$(GIT_COMMIT)                    \
	    --set applicationRevisionLimit=6                 \
	    --wait vela-rollout                              \
	    --set optimize.disableComponentRevision=false              \
	    ./runtime/rollout/charts ||						 \
	echo "no worker cluster"					   		 \



.PHONY: e2e-api-test
e2e-api-test:
	# Run e2e test
	ginkgo -v -skipPackage capability,setup,application -r e2e
	ginkgo -v -r e2e/application


.PHONY: e2e-test
e2e-test:
	# Run e2e test
	ginkgo -v  --skip="rollout related e2e-test." ./test/e2e-test
	@$(OK) tests pass

.PHONY: e2e-addon-test
e2e-addon-test:
	cp bin/vela /tmp/
	ginkgo -v ./test/e2e-addon-test
	@$(OK) tests pass

.PHONY: e2e-rollout-test
e2e-rollout-test:
	ginkgo -v  --focus="rollout related e2e-test." ./test/e2e-test
	@$(OK) tests pass

.PHONY: e2e-multicluster-test
e2e-multicluster-test:
	go test -v -coverpkg=./... -coverprofile=/tmp/e2e_multicluster_test.out ./test/e2e-multicluster-test
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