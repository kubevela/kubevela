# Install KubeVela

## Prerequisites
- Kubernete cluster which is v1.15.0 or greater
- kubectl current context is configured for the target cluster install
  - ```kubectl config current-context```

### Minikube

> TODO enable ingress controller

### KinD

> TODO anything need to do?

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
grafana              	monitoring  	1
...
```

## Dependencies

We have installed the following dependency components along with Vela server component:

- [Prometheus](https://prometheus-community.github.io/helm-charts/) & [Grafana](https://github.com/grafana/helm-charts/tree/main/charts/grafana)
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

Then clean up CRDs:

```
$ kubectl delete crd workloaddefinitions.core.oam.dev traitdefinitions.core.oam.dev  scopedefinitions.core.oam.dev
```
