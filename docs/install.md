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

Install KubeVela server component:

```console
vela install
```

## Verify

> TODO Paste a output of successful installation here.

## Dependencies

> TODO Describe how vela install handle Prometheus & Grafana, Flagger and KEDA, and what if user want to replace them with his own version. (It's fine to say KEDA, Flagger is a temporary fork and we will ship the fixes to upstreams very soon)


## Clean Up

Clean up KubeVela server component:

```console
helm uninstall -n vela-system kubevela
rm -r ~/.vela
```
