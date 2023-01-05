#! /bin/bash
legacyDefs=("apply-deployment" "apply-terraform-config" "apply-terraform-provider" "clean-jobs" "request" "vela-cli")

for item in ${legacyDefs[@]}; do
  val=$(kubectl get workflowstepdefinition ${item} -n vela-system -ojsonpath='{.metadata.annotations.meta\.helm\.sh/release-name}' 2>/dev/null)
  [[ $? -ne 0 ]] && echo "Skipping ${item} step definition" && continue
  if [[ $val != kubevela ]]; then
    echo "Patching ${item}"
    kubectl patch -n vela-system workflowstepdefinition ${item} --type=merge -p '{"metadata":{"annotations":{"meta.helm.sh/release-name":"kubevela","meta.helm.sh/release-namespace":"vela-system"},"labels":{"app.kubernetes.io/managed-by":"Helm"}}}'
    echo "Successfully take over the ${item} step definition"
  fi
done

legacyViews=("component-pod-view" "component-service-view")

for item in ${legacyViews[@]}; do
  val=$(kubectl get configMap ${item} -n vela-system -ojsonpath='{.metadata.annotations.meta\.helm\.sh/release-name}'  2>/dev/null)
  [[ $? -ne 0 ]] && echo "Skipping ${item} view" && continue
  if [[ $val != kubevela ]]; then
    kubectl patch -n vela-system configMap ${item} --type=merge -p '{"metadata":{"annotations":{"meta.helm.sh/release-name":"kubevela","meta.helm.sh/release-namespace":"vela-system"},"labels":{"app.kubernetes.io/managed-by":"Helm"}}}'
    echo "Successfully take over the ${item} view"
  fi
done