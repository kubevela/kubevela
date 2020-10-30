# Install KubeVela

## Prerequisites

- Kubernetes cluster >= v1.15.0
- kubectl installed and configured

You may pick either Minikube or KinD as local cluster testing option.

### Minikube

> TODO enable ingress controller

### KinD

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

## Get KubeVela

> TODO please give a copy-paste friendly shell instead of instructions

1. Download the latest `vela` binary from the [releases page](https://github.com/oam-dev/kubevela/releases).
2. Unpack the `vela` binary and add it to `$PATH` to get started.

```console
$ sudo mv ./vela /usr/local/bin/vela
```

## Initialize KubeVela

Run:

```console
$ vela install
```

This will install KubeVela server component and its dependency components.

## Verify

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
ingress-nginx        	vela-system 	1
kube-prometheus-stack	monitoring  	1
...
```

## Dependencies

We have installed the following dependency components along with Vela server component:

- [Prometheus Stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
- [Cert-manager](https://cert-manager.io/)
- [Ingress-nginx](https://github.com/kubernetes/ingress-nginx/)
- [Flagger](https://flagger.app/)

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
