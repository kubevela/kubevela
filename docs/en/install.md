# Install KubeVela

## 1. Setup local k8s cluster

- Kubernetes cluster >= v1.15.0
- kubectl installed and configured

You may pick either Minikube or KinD as local cluster testing option.

<!-- tabs:start -->

#### ** Minikube **

Follow the minikube [installation guide](https://minikube.sigs.k8s.io/docs/start/).

Once minikube is installed, create a cluster:

```console
$ minikube start
```

Install ingress:

```console
$ minikube addons enable ingress
```

#### ** KinD **

Follow [this guide](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) to install kind.

Then spins up a kind cluster:

```console
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
```console
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml
```

<!-- tabs:end -->

## 2. Get KubeVela

1. Download the latest `vela` binary from the [releases page](https://github.com/oam-dev/kubevela/releases).
2. Unpack the `vela` binary and add it to `$PATH` to get started.

```console
$ sudo mv ./vela /usr/local/bin/vela
```

## 3. Initialize KubeVela

Run:

```console
$ vela install
```

This will install KubeVela server component and its dependency components.

## 4. Verify

Check Vela Helm Chart has been installed:
```
$ helm list -n vela-system
NAME    	NAMESPACE  	REVISION	...
kubevela	vela-system	1       	...
```

Later on, check that the dependency components has been installed:
```
$ helm list --all-namespaces
NAME                 	NAMESPACE   	REVISION	...
cert-manager         	cert-manager	1       	...
flagger              	vela-system 	1
kube-prometheus-stack	monitoring  	1
...
```

**Voila!** You are all set to go.

## Clean Up

Run:

```console
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

## [Optional] Add/Update Dependencies

We have installed the following dependency components along with Vela server component:

- [Prometheus Stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
- [Cert-manager](https://cert-manager.io/)
- [Flagger](https://flagger.app/)

> [!NOTE]
> If you are not using minikube or kind, please make sure to [install ingress-nginx](https://kubernetes.github.io/ingress-nginx/deploy/) by yourself.

The config has been saved in a ConfigMap in "vela-system/vela-config":

```
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
```
$ kubectl -n vela-system edit cm vela-config
...

$ helm uninstall -n vela-system kubevela
$ helm install -n vela-system kubevela
```