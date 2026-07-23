# Addon-as-component StateKeep audit

- Date: 2026-07-16T15:29:37.763Z
- Cluster: `k3d-kubevela`
- Controller: current IntelliJ debug process, `--application-re-sync-period=20s`
- Scope: the 25 addons classified PASS/PASS in the 2026-07-09 differential report
- Existing state preserved: `comp-terraform-aws` and `addon-terraform-aws`
- Result: 23/25 completed the deletion/recreation check; 2/25 were blocked by invalid auxiliary manifests before deletion
- Recreation failures among addons that reached the deletion step: 0

## Method

1. Apply `localtest/addon-component/<addon>.yaml`.
2. Require both `comp-<addon>` and `addon-<addon>` to reach `running`.
3. Select the current versioned ResourceTracker using the inner Application generation.
4. Select a raw-backed resource at random from the lowest-risk available tier. CRDs, Namespaces, Applications, persistent volumes, webhooks, and APIServices are excluded.
5. Delete the resource and require the same kind/namespace/name to return with a different UID within 120 seconds.
6. Remove task-created Applications before proceeding to the next addon.

## Results

| Addon | Install | Deleted tracked resource | Old UID | New UID | Recreated | Notes |
|---|---|---|---|---|---:|---|
| cert-manager | PASS | `HelmRepository/cert-manager/cert-manager` | `c7d42d33-7a39-4235-8f5b-34fc5ac11154` | `0bfaba4f-313f-4f12-9fcf-a335983a2814` | 11s | Temporary helm definition prerequisite in vela-system; standalone install first failed because fluxcd dependency was not installed and its injected definition was relocated to flux-system. |
| chartmuseum | PASS | `Secret/vela-system/chartmuseum-gcs-credential` | `41503c98-761e-4f85-9152-21956adf2598` | `8f186637-9f21-46ca-996f-29252bd43ce7` | 19s | apiVersion=v1 tracker=addon-chartmuseum-v1-vela-system inner-status=running |
| ingress-nginx | PASS | `Secret/vela-system/addon-secret-ingress-nginx` | `63233649-0113-413e-af1f-aa4ee3818714` | `eb890661-4b0d-413f-8fa2-3e1256aba2f1` | 19s | apiVersion=v1 tracker=addon-ingress-nginx-v1-vela-system inner-status=running |
| keda | PASS | `TraitDefinition/kube-system/keda-auto-scaler` | `008de53a-8669-4642-b2cb-1e10d58ec8a9` | `e73737fd-3c72-45fd-bb75-5699f414eda0` | 18s | apiVersion=core.oam.dev/v1beta1 tracker=addon-keda-v1-vela-system inner-status=running |
| kube-trigger | PASS | `Secret/vela-system/addon-secret-kube-trigger` | `5bee915d-cceb-487e-923f-294437452755` | `967f17db-ce3e-46f1-a3dc-31fed1327db5` | 17s | apiVersion=v1 tracker=addon-kube-trigger-v1-vela-system inner-status=running |
| kubevela-io | PASS | `Secret/vela-system/addon-secret-kubevela-io` | `bc683e77-c9f6-4bc2-9a18-0f76649d6e97` | `7ff57da0-ccda-4709-93a1-7d49f507b9f9` | 11s | apiVersion=v1 tracker=addon-kubevela-io-v1-vela-system inner-status=running |
| model-training | PASS | `ComponentDefinition/vela-system/jupyter-notebook` | `00a40618-948d-4a1c-958d-b85b91c21000` | `9fe5a20a-d6d8-4cb4-9cb7-0b2627b51fb9` | 17s | apiVersion=core.oam.dev/v1beta1 tracker=addon-model-training-v1-vela-system inner-status=running |
| mysql-exporter | INSTALL_FAIL | - | - | - | - | Resolved version 0.0.1. Addon component failed pre-dispatch because ComponentDefinition "mysql-exporter-server" is v1beta1 but lacks required spec.workload. Imperative enable also exited 1, while addon-mysql-exporter was already running, explaining the old false PASS. |
| netlify | PASS | `ComponentDefinition/vela-system/netlify` | `b3b79e49-edcf-49d0-9716-4293df7e39c3` | `52025b99-ea82-47d0-830d-70722058c35b` | 20s | apiVersion=core.oam.dev/v1beta1 tracker=addon-netlify-v1-vela-system inner-status=running |
| node-exporter | PASS | `Secret/o11y-system/addon-secret-node-exporter` | `c9ee4bff-613d-43ea-ba0a-156b9616c621` | `cef53037-56b7-4772-9cc9-37352ba62b60` | 21s | apiVersion=v1 tracker=addon-node-exporter-v1-vela-system inner-status=running |
| o11y-definitions | PASS | `WorkflowStepDefinition/vela-system/install-datasource-from-config` | `874fbc68-e0d5-41ec-a18c-3ae9cd4ba6aa` | `d3cfaa9a-ec28-4c7e-91bd-1d74daf770cf` | 15s | apiVersion=core.oam.dev/v1beta1 tracker=addon-o11y-definitions-v1-vela-system inner-status=running |
| ocm-hub-control-plane | PASS | `Secret/vela-system/addon-secret-ocm-hub-control-plane` | `3b42752f-bdb8-46c9-8865-3b116ad2e41a` | `66fd019b-d7ef-4567-aacc-aa7dcb051dcb` | 19s | apiVersion=v1 tracker=addon-ocm-hub-control-plane-v1-vela-system inner-status=running |
| terraform | PASS | `Secret/vela-system/addon-secret-terraform` | `5869a25f-30fb-422d-b6a7-1f9b9c51c9a0` | `00c59a80-f4dc-49f4-a26d-6ed9ef50080a` | 18s | apiVersion=v1 tracker=addon-terraform-v1-vela-system inner-status=running |
| terraform-alibaba | PASS | `ComponentDefinition/vela-system/alibaba-ram-fc` | `0ebf1ebc-5f4c-4f60-b1c3-113034b4040e` | `3cae8f37-2d75-4762-9b40-a9d204added5` | 13s | apiVersion=core.oam.dev/v1beta1 tracker=addon-terraform-alibaba-v1-vela-system inner-status=running |
| terraform-aws | PASS | `ComponentDefinition/vela-system/aws-iam-nofile` | `3987d7e2-1262-48b9-b37f-0e42c53fcd72` | `791721fe-9346-41de-a95b-82ae3ad832aa` | 9s | apiVersion=core.oam.dev/v1beta1 tracker=addon-terraform-aws-v1-vela-system inner-status=running |
| terraform-azure | INSTALL_FAIL | - | - | - | - | Resolved version 1.0.3. Addon component failed pre-dispatch because the package contains ComponentDefinition core.oam.dev/v1alpha2 (azure-storage-account), an unserved API. Imperative enable also exited 1, while addon-terraform-azure was already running, explaining the old false PASS. |
| terraform-baidu | PASS | `ConfigMap/vela-system/config-template-terraform-baidu` | `1964c6d6-51a9-4c2c-9873-508de54f23be` | `ecdc0c78-a2fc-4ea6-800f-f801656e4353` | 15s | apiVersion=v1 tracker=addon-terraform-baidu-v1-vela-system inner-status=running |
| terraform-ec | PASS | `Secret/vela-system/addon-secret-terraform-ec` | `2684265c-4f98-4b2d-863f-94b734cda963` | `c445bf12-e0c6-4086-ba8e-462a86a8e705` | 15s | apiVersion=v1 tracker=addon-terraform-ec-v1-vela-system inner-status=running |
| terraform-gcp | PASS | `ComponentDefinition/vela-system/gcp-memorystore-redis` | `89ae2ff1-98dc-4356-9b6f-58c1eb41e411` | `5db221eb-6fd1-4090-b5ef-b314696df8ba` | 11s | apiVersion=core.oam.dev/v1beta1 tracker=addon-terraform-gcp-v1-vela-system inner-status=running |
| terraform-tencent | PASS | `ConfigMap/vela-system/config-template-terraform-tencent` | `32845d03-fb0a-4f27-94a4-01adbeb19770` | `4d5f32ad-77ee-4693-9101-5583822fdd5b` | 15s | apiVersion=v1 tracker=addon-terraform-tencent-v1-vela-system inner-status=running |
| terraform-ucloud | PASS | `ConfigMap/vela-system/config-template-terraform-ucloud` | `53edd54e-defb-4264-8f88-65063f4c3589` | `43f38913-49e5-4eb4-98a7-79cd44ba8ef7` | 15s | apiVersion=v1 tracker=addon-terraform-ucloud-v1-vela-system inner-status=running |
| trivy-operator | PASS | `WorkflowStepDefinition/trivy-system/trivy-check` | `53a29aa9-cb57-434d-bcfc-84ee3fc46f91` | `672eaba3-7c8b-46e0-9747-8493acab6de5` | 18s | apiVersion=core.oam.dev/v1beta1 tracker=addon-trivy-operator-v1-vela-system inner-status=running |
| vegeta | PASS | `TraitDefinition/vela-system/vegeta` | `0905497b-cf7f-42eb-bf89-93ac09d399a0` | `daf668f6-e0c0-466e-aa18-fe62f0fd88f6` | 13s | apiVersion=core.oam.dev/v1beta1 tracker=addon-vegeta-v1-vela-system inner-status=running |
| vela-prism | PASS | `ClusterRoleBinding/cluster-scoped/vela-prism:prism-cluster-access-rolebinding` | `0f3bf68b-4a73-4005-8e33-3f5e4214b5b1` | `598fcb16-2b19-45ec-a38a-9e1e01a18c25` | 17s | apiVersion=rbac.authorization.k8s.io/v1 tracker=addon-vela-prism-v1-vela-system inner-status=running |
| victoria-metric | PASS | `Secret/vela-system/vm-cluster` | `3765d45d-ef33-4dab-b34e-956de3259074` | `033013e9-93aa-4315-bb14-17839cc0fc94` | 16s | Dependency-assisted: imperative victoria-metric enable left config-template-prometheus-server in vela-system; apiVersion=v1 tracker=addon-victoria-metric-v1-vela-system inner-status=running |

## Hidden findings

### 1. Component mode does not install addon dependencies

`vela addon enable` installs an addon's dependencies before installing the addon itself. The component renderer in `pkg/addon/service/renderer.go` does not do that. It fetches and renders only the addon named in the component.

This caused two failures during the audit:

- `cert-manager` needs the `helm` ComponentDefinition supplied by `fluxcd`. Without it, the child Application could not render.
- `victoria-metric` needs `config-template-prometheus-server`. Without that ConfigMap, its `prometheus-server-register` workflow step failed.

Both addons passed the deletion and recreation test after those dependencies were added manually. Their PASS results prove that StateKeep works once the required resources exist. They do not prove that either addon can be installed as a standalone addon component.

### 2. Topology policies also move auxiliary resources

The renderer adds definitions, ConfigMaps, and other auxiliary resources as components of the child Application. An addon topology policy then applies to those new components as well as to the addon's workloads.

For example, `fluxcd` 3.0.2 targets the `flux-system` namespace. Its `ComponentDefinition/helm` was therefore created in `flux-system`. Applications in `vela-system`, including `cert-manager`, look for that definition in `vela-system`, so installing `fluxcd` as a component still did not make `cert-manager` work.

The same placement behavior put `TraitDefinition/keda-auto-scaler` in `kube-system` and `WorkflowStepDefinition/trivy-check` in `trivy-system`. Workloads should follow the addon's topology policy, but control-plane definitions and configuration should stay in their intended namespace.

### 3. Two PASS results in the old report were not complete installs

The old baseline harness checked whether the child Application reached `running`. It did not treat a nonzero exit from `vela addon enable` as a failure when the child Application already existed.

That sequence matters because the imperative installer creates the child Application before it applies the auxiliary resources. The Application can report `running` even when a later auxiliary resource fails:

- `mysql-exporter` 0.0.1 created a running Application, but `vela addon enable` exited with code 1 because `ComponentDefinition/mysql-exporter-server` has no required `spec.workload` field.
- `terraform-azure` 1.0.3 created a running Application, but the command exited with code 1 because the package contains `core.oam.dev/v1alpha2` ComponentDefinitions. That API version is not served by the cluster.

Those two rows were partial installs, not real PASS results. Component mode makes the problem visible because the auxiliary resources are part of the child Application workflow, causing that workflow to fail.

### 4. Two child Applications returned during cleanup

After their wrapper Applications were deleted, `addon-o11y-definitions` and `addon-vela-prism` disappeared and then returned once. They were removed manually after their ResourceTrackers were gone and did not return again.

This looks like a race between wrapper deletion, ResourceTracker cleanup, and StateKeep reconciliation. It happened once during this audit, so it needs a focused reproduction before changing controller code.

## Cleanup

All task-created wrapper/inner Applications and temporary prerequisite resources were removed. The cluster was returned to its initial Application set: `comp-terraform-aws` and `addon-terraform-aws`.
