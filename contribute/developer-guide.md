# Developer guide

This guide helps you get started developing KubeVela.

## Prerequisites

1. Golang version 1.16+
2. Kubernetes version v1.18+ with `~/.kube/config` configured.
3. ginkgo 1.14.0+ (just for [E2E test](./developer-guide.md#e2e-test))
4. golangci-lint 1.38.0+, it will install automatically if you run `make`, you can [install it manually](https://golangci-lint.run/usage/install/#local-installation) if the installation is too slow.
5. kubebuilder v2.3.0+

<details>
  <summary>Install Kubebuilder manually</summary>

linux:
```
wget https://github.com/kubernetes-sigs/kubebuilder/releases/download/v2.3.1/kubebuilder_2.3.1_linux_amd64.tar.gz
tar -zxvf  kubebuilder_2.3.1_linux_amd64.tar.gz
mkdir -p /usr/local/kubebuilder/bin
sudo mv kubebuilder_2.3.1_linux_amd64/bin/* /usr/local/kubebuilder/bin
```

macOS:
```
wget https://github.com/kubernetes-sigs/kubebuilder/releases/download/v2.3.1/kubebuilder_2.3.1_darwin_amd64.tar.gz
tar -zxvf  kubebuilder_2.3.1_darwin_amd64.tar.gz
mkdir -p /usr/local/kubebuilder/bin
sudo mv kubebuilder_2.3.1_darwin_amd64/bin/* /usr/local/kubebuilder/bin
```

</details>

You may also be interested with KubeVela's [design](https://github.com/oam-dev/kubevela/tree/master/design/vela-core) before diving into its code.

## Build

* Clone this project

```shell script
git clone git@github.com:oam-dev/kubevela.git
```

KubeVela includes two parts, `vela core` and `vela cli`.

- The `vela core` is actually a K8s controller, it will watch OAM Spec CRD and deploy resources.
- The `vela cli` is a command line tool that can build, run apps(with the help of `vela core`).

For local development, we probably need to build both of them.

* Build Vela CLI

```shell script
make
```

After the vela cli built successfully, `make` command will create `vela` binary to `bin/` under the project.

* Configure `vela` binary to System PATH

```shell script
export PATH=$PATH:/your/path/to/project/kubevela/bin
```

Then you can use `vela` command directly.

* Build Vela Core

```shell script
make manager
```

* Run Vela Core

Firstly make sure your cluster has CRDs, below is the command that can help install all CRDs.

```shell script
make core-install
```

To ensure you have created vela-system namespace and install definitions of necessary module.
you can run the command:
```shell script
make check-install-def
```

And then run locally:
```shell script
make core-run
```

This command will run controller locally, it will use your local KubeConfig which means you need to have a k8s cluster
locally. If you don't have a one, we suggest that you could setup up a cluster with [kind](https://kind.sigs.k8s.io/).

When you're developing `vela-core`, make sure the controller installed by helm chart is not running.
Otherwise, it will conflict with your local running controller.

You can check and uninstall it by using helm.

```shell script
helm list -A
helm uninstall -n vela-system kubevela
```

## Use

You can try use your local built binaries follow [the documentation](https://kubevela.io/docs/quick-start).

## Testing

### Unit test

```shell script
make test
```

### E2E test

**Before e2e test start, make sure you have vela-core running.**

```shell script
make core-run
```

Start to test.

```
make e2e-test
```

## Contribute Docs

Please read [the documentation](https://github.com/oam-dev/kubevela/tree/master/docs/README.md) before contributing to the docs.

- Build docs

```shell script
make docs-build
```

- Local development and preview

```shell script
make docs-start
```

## Next steps

* Read our [code conventions](coding-conventions.md)
* Learn how to [Create a pull request](create-pull-request.md)
