# Install KubeVela

## Prerequisites
- ubernete cluster which is v1.15.0 or greater
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

```console
$ vela install
```
This command will install KubeVela server components in your Kubernetes cluster.

## Verify

> TODO Paste a output of successful installation here.


## Clean Up

```console
$ helm uninstall kubevela -n vela-system
release "kubevela" uninstalled
```

```console
$ kubectl delete crd workloaddefinitions.core.oam.dev traitdefinitions.core.oam.dev  scopedefinitions.core.oam.dev
customresourcedefinition.apiextensions.k8s.io "workloaddefinitions.core.oam.dev" deleted
customresourcedefinition.apiextensions.k8s.io "traitdefinitions.core.oam.dev" deleted
```

```console
$ rm -r ~/.vela
```