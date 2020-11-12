# Install KubeVela

## 1. Setup Kubernetes cluster

Requirements:
- Kubernetes cluster >= v1.15.0
- kubectl installed and configured

You may pick either Minikube or KinD as local cluster testing option.

> NOTE: If you are not using minikube or kind, please make sure to [install or enable ingress-nginx](https://kubernetes.github.io/ingress-nginx/deploy/) by yourself.

<details><summary>Minikube</summary>
  <p>

Follow the minikube [installation guide](https://minikube.sigs.k8s.io/docs/start/).

Once minikube is installed, create a cluster:

```bash
$ minikube start
```

Install ingress:

```bash
$ minikube addons enable ingress
```
  <p>
</details>  

<details><summary>KinD</summary>
  <p>

Follow [this guide](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) to install kind.

Then spins up a kind cluster:

```bash
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF
```

Then install [ingress for kind](https://kind.sigs.k8s.io/docs/user/ingress/#ingress-nginx):
```bash
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml
```
  <p>
</details> 

## 2. Get KubeVela

1. Download the latest `vela` binary from the [releases page](https://github.com/oam-dev/kubevela/releases).
2. Unpack the `vela` binary and add it to `$PATH` to get started.

```bash
$ sudo mv ./vela /usr/local/bin/vela
```

## 3. Initialize KubeVela

Run:

```bash
$ vela install
```

This will install KubeVela server component and its dependency components.

<details><summary>(Advanced) Verify Installation Manually</summary>
  <p>
  Check Vela Helm Chart has been installed:

  ```console
  $ helm list -n vela-system
  NAME      NAMESPACE   REVISION  ...
  kubevela  vela-system 1         ...
  ```

  Later on, check that all dependency components has been installed (they will need 5-10 minutes to show up):

  ```console
  $ helm list --all-namespaces
  NAME                  NAMESPACE   REVISION  UPDATED                               STATUS    CHART                       APP VERSION
  flagger               vela-system 1         2020-11-10 18:47:14.0829416 +0000 UTC deployed  flagger-1.1.0               1.1.0
  keda                  keda        1         2020-11-10 18:45:15.6981827 +0000 UTC deployed  keda-2.0.0-rc3              2.0.0-rc2
  kube-prometheus-stack monitoring  1         2020-11-10 18:45:37.9608079 +0000 UTC deployed  kube-prometheus-stack-9.4.4 0.38.1
  kubevela              vela-system 1         2020-11-10 10:44:20.663582 -0800 PST  deployed
  ```

  > We will introduce a `vela system health` command to check the dependencies in the future.
  </p>
</details>

<details><summary>(Advanced) Customize Your Installation</summary>
  <p>
  We have installed the following dependency components along with Vela server component:

  - [Prometheus Stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
  - [Cert-manager](https://cert-manager.io/)
  - [Flagger](https://flagger.app/)
  - [KEDA](https://keda.sh/)

  The config has been saved in a ConfigMap in "vela-system/vela-config":

  ```console
  $ kubectl -n vela-system get cm vela-config -o yaml
  apiVersion: v1
  data:
    certificates.cert-manager.io: |
      {
        "repo": "jetstack",
        "urL": "https://charts.jetstack.io",
        "name": "cert-manager",
        "namespace": "cert-manager",
        "version": "1.0.3"
      }
    flagger.app: |
    ...
  kind: ConfigMap
  ```

  User can specify their own dependencies by editing the `vela-config` ConfigMap.
  Currently adding new chart or updating existing chart requires redeploying Vela:

  ```console
  $ kubectl -n vela-system edit cm vela-config
  ...

  $ helm uninstall -n vela-system kubevela
  $ helm install -n vela-system kubevela
  ```
  </p>
</details>

## 4. (Optional) Clean Up

<details>

Run:

```bash
$ helm uninstall -n vela-system kubevela
$ rm -r ~/.vela
```

This will uninstall KubeVela server component and its dependency components.
This also cleans up local CLI cache.

Then clean up CRDs (CRDs are not removed via helm by default):

```
$ kubectl delete crd \
  applicationconfigurations.core.oam.dev \
  applicationdeployments.core.oam.dev \
  autoscalers.standard.oam.dev \
  certificaterequests.cert-manager.io \
  certificates.cert-manager.io \
  challenges.acme.cert-manager.io \
  clusterissuers.cert-manager.io \
  components.core.oam.dev \
  containerizedworkloads.core.oam.dev \
  healthscopes.core.oam.dev \
  issuers.cert-manager.io \
  manualscalertraits.core.oam.dev \
  metricstraits.standard.oam.dev \
  orders.acme.cert-manager.io \
  podspecworkloads.standard.oam.dev \
  routes.standard.oam.dev \
  scopedefinitions.core.oam.dev \
  servicemonitors.monitoring.coreos.com \
  traitdefinitions.core.oam.dev \
  workloaddefinitions.core.oam.dev
```
</details>