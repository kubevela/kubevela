---
title:  Advanced Topics for Installation
---

## Install KubeVela with cert-manager

KubeVela can use cert-manager generate certs for your application if it's available. Note that you need to install cert-manager **before** the KubeVela chart.

```shell script
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager --namespace cert-manager --version v1.2.0 --create-namespace --set installCRDs=true
```

Install kubevela with enabled certmanager:
```shell script
helm install --create-namespace -n vela-system --set admissionWebhooks.certManager.enabled=true kubevela kubevela/vela-core
```

## Install Pre-release
    
Add flag `--devel` in command `helm search` to choose a pre-release
version in format `<next_version>-rc-master`. It means a release candidate version build on `master` branch,
such as `0.4.0-rc-master`.

```shell script
helm search repo kubevela/vela-core -l --devel
```
```console
    NAME                      CHART VERSION         APP VERSION           DESCRIPTION
    kubevela/vela-core        0.4.0-rc-master         0.4.0-rc-master         A Helm chart for KubeVela core
    kubevela/vela-core        0.3.2                   0.3.2                   A Helm chart for KubeVela core
    kubevela/vela-core        0.3.1                 0.3.1                 A Helm chart for KubeVela core
```

And try the following command to install it.

```shell script
helm install --create-namespace -n vela-system kubevela kubevela/vela-core --version <next_version>-rc-master
```
```console
NAME: kubevela
LAST DEPLOYED: Thu Apr  1 19:41:30 2021
NAMESPACE: vela-system
STATUS: deployed
REVISION: 1
NOTES:
Welcome to use the KubeVela! Enjoy your shipping application journey!
```

## Upgrade

### Step 1. Update Helm repo


You can explore the newly released chart versions of KubeVela by run:

```shell
helm repo update
helm search repo kubevela/vela-core -l
```

### Step 2. Upgrade KubeVela CRDs

```shell
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_componentdefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_workloaddefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_traitdefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_applications.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_approllouts.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_applicationrevisions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_scopedefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_appdeployments.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_applicationcontexts.yaml
```

> Tips: If you see errors like `* is invalid: spec.scope: Invalid value: "Namespaced": filed is immutable`. Please delete the CRD which reports error and re-apply the kubevela crds.

```shell
 kubectl delete crd \
  scopedefinitions.core.oam.dev \
  traitdefinitions.core.oam.dev \
  workloaddefinitions.core.oam.dev
```

### Step 3. Upgrade KubeVela Helm chart

```shell
helm upgrade --install --create-namespace --namespace vela-system  kubevela kubevela/vela-core --version <the_new_version>
```

## Clean Up

Run:

```shell script
helm uninstall -n vela-system kubevela
rm -r ~/.vela
```

This will uninstall KubeVela server component and its dependency components.
This also cleans up local CLI cache.

Then clean up CRDs (CRDs are not removed via helm by default):

```shell script
 kubectl delete crd \
  appdeployments.core.oam.dev \
  applicationconfigurations.core.oam.dev \
  applicationcontexts.core.oam.dev \
  applicationrevisions.core.oam.dev \
  applications.core.oam.dev \
  approllouts.core.oam.dev \
  componentdefinitions.core.oam.dev \
  components.core.oam.dev \
  containerizedworkloads.core.oam.dev \
  healthscopes.core.oam.dev \
  manualscalertraits.core.oam.dev \
  podspecworkloads.standard.oam.dev \
  scopedefinitions.core.oam.dev \
  traitdefinitions.core.oam.dev \
  workloaddefinitions.core.oam.dev
```