
e2e-setup-core:
	sh ./hack/e2e/modify_charts.sh
	helm upgrade --install --create-namespace --namespace vela-system --set image.pullPolicy=IfNotPresent --set image.repository=vela-core-test --set applicationRevisionLimit=5 --set dependCheckWait=10s --set image.tag=$(GIT_COMMIT) --wait kubevela ./charts/vela-core
	kubectl wait --for=condition=Available deployment/kubevela-vela-core -n vela-system --timeout=180s
	go run ./e2e/addon/mock &

setup-runtime-e2e-cluster:
	helm upgrade --install --create-namespace --namespace vela-system --kubeconfig=$(RUNTIME_CLUSTER_CONFIG) --set image.pullPolicy=IfNotPresent --set image.repository=vela-runtime-rollout-test --set image.tag=$(GIT_COMMIT) --wait vela-rollout ./runtime/rollout/charts

e2e-setup:
	helm install kruise https://github.com/openkruise/kruise/releases/download/v0.9.0/kruise-chart.tgz --set featureGates="PreDownloadImageForInPlaceUpdate=true"
	sh ./hack/e2e/modify_charts.sh
	helm upgrade --install --create-namespace --namespace vela-system --set image.pullPolicy=IfNotPresent --set image.repository=vela-core-test --set applicationRevisionLimit=5 --set dependCheckWait=10s --set image.tag=$(GIT_COMMIT) --wait kubevela ./charts/vela-core
	helm upgrade --install --create-namespace --namespace oam-runtime-system --set image.pullPolicy=IfNotPresent --set image.repository=vela-core-test --set dependCheckWait=10s --set image.tag=$(GIT_COMMIT) --wait oam-runtime ./charts/oam-runtime
	go run ./e2e/addon/mock &
	bin/vela addon enable fluxcd
	bin/vela addon enable terraform
	bin/vela addon enable terraform-alibaba ALICLOUD_ACCESS_KEY=xxx ALICLOUD_SECRET_KEY=yyy ALICLOUD_REGION=cn-beijing
	ginkgo version
	ginkgo -v -r e2e/setup

	timeout 600s bash -c -- 'while true; do kubectl get ns flux-system; if [ $$? -eq 0 ] ; then break; else sleep 5; fi;done'
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=vela-core,app.kubernetes.io/instance=kubevela -n vela-system --timeout=600s
	kubectl wait --for=condition=Ready pod -l app=source-controller -n flux-system --timeout=600s
	kubectl wait --for=condition=Ready pod -l app=helm-controller -n flux-system --timeout=600s

e2e-api-test:
	# Run e2e test
	ginkgo -v -skipPackage capability,setup,application -r e2e
	ginkgo -v -r e2e/application

e2e-apiserver-test: build-swagger
	git --no-pager diff
	git diff --quiet || ($(ERR) please run 'make build-swagger' to include all API changes && false)
	go run ./e2e/addon/mock &
	go test -v -coverpkg=./... -coverprofile=/tmp/e2e_apiserver_test.out ./test/e2e-apiserver-test
	@$(OK) tests pass

e2e-test:
	# Run e2e test
	ginkgo -v  --skip="rollout related e2e-test." ./test/e2e-test
	@$(OK) tests pass

e2e-addon-test:
	cp bin/vela /tmp/
	ginkgo -v ./test/e2e-addon-test
	@$(OK) tests pass

e2e-rollout-test:
	ginkgo -v  --focus="rollout related e2e-test." ./test/e2e-test
	@$(OK) tests pass

e2e-multicluster-test:
	go test -v -coverpkg=./... -coverprofile=/tmp/e2e_multicluster_test.out ./test/e2e-multicluster-test
	@$(OK) tests pass


e2e-cleanup:
	# Clean up
	rm -rf ~/.vela

end-e2e-core:
	sh ./hack/e2e/end_e2e_core.sh

end-e2e:
	sh ./hack/e2e/end_e2e.sh