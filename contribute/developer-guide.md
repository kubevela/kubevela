# Developer guide

This guide helps you get started developing KubeVela.

## Prerequisites

1. Golang version 1.17+
2. Kubernetes version v1.20+ with `~/.kube/config` configured.
3. ginkgo 1.14.0+ (just for [E2E test](./developer-guide.md#e2e-test))
4. golangci-lint 1.38.0+, it will install automatically if you run `make`, you can [install it manually](https://golangci-lint.run/usage/install/#local-installation) if the installation is too slow.
5. kubebuilder v3.1.0+ and you need to manually install the dependency tools for unit test.
6. [CUE binary](https://github.com/cue-lang/cue/releases) v0.3.0+

<details>
  <summary>Install Kubebuilder manually</summary>

linux:

```
wget https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-1.21.2-linux-amd64.tar.gz
tar -zxvf  kubebuilder-tools-1.21.2-linux-amd64.tar.gz
mkdir -p /usr/local/kubebuilder/bin
sudo mv kubebuilder/bin/* /usr/local/kubebuilder/bin
```

macOS:

```
wget https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-1.21.2-darwin-amd64.tar.gz
tar -zxvf  kubebuilder-tools-1.21.2-darwin-amd64.tar.gz
mkdir -p /usr/local/kubebuilder/bin
sudo mv kubebuilder/bin/* /usr/local/kubebuilder/bin
```

For other OS or system architecture, please refer to https://storage.googleapis.com/kubebuilder-tools/

</details>

You may also be interested with KubeVela's [design](https://github.com/oam-dev/kubevela/tree/master/design/vela-core) before diving into its code.

## Build

- Clone this project

```shell script
git clone git@github.com:oam-dev/kubevela.git
```

KubeVela includes two parts, `vela core` and `vela cli`.

- The `vela core` is actually a K8s controller, it will watch OAM Spec CRD and deploy resources.
- The `vela cli` is a command line tool that can build, run apps(with the help of `vela core`).

For local development, we probably need to build both of them.

- Build Vela CLI

```shell script
make
```

After the vela cli built successfully, `make` command will create `vela` binary to `bin/` under the project.

- Configure `vela` binary to System PATH

```shell script
export PATH=$PATH:/your/path/to/project/kubevela/bin
```

Then you can use `vela` command directly.

- Build Vela Core

```shell script
make manager
```

- Run Vela Core

Firstly make sure your cluster has CRDs, below is the command that can help install all CRDs.

```shell script
make core-install
```

To ensure you have created vela-system namespace and install definitions of necessary module.
you can run the command:

```shell script
make def-install
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

To execute the unit test of the API module, the mongodb service needs to exist locally.

```shell script
make unit-test-apiserver
```

### E2E test

**Before e2e test start, make sure you have vela-core running.**

```shell script
make core-run
```

Start to test.

```shell script
make e2e-test
```

## Contribute apiserver and [velaux](https://github.com/oam-dev/velaux)

Before start, please make sure you have already started the vela controller environment.

```shell
make run-apiserver
```

By default, the apiserver will serving at "0.0.0.0:8000".

Get the velaux code by:

```shell
git clone git@github.com:oam-dev/velaux.git
```

Configure the apiserver address:

```shell
cd velaux
echo "BASE_DOMAIN='http://127.0.0.1:8000'" > .env
```

Make sure you have installed [yarn](https://classic.yarnpkg.com/en/docs/install).

```shell
yarn install
yarn start
```

To execute the e2e test of the API module, the mongodb service needs to exist locally.

```shell script
# save your config
mv ~/.kube/config  ~/.kube/config.save

kind create cluster --image kindest/node:v1.20.7@sha256:688fba5ce6b825be62a7c7fe1415b35da2bdfbb5a69227c499ea4cc0008661ca --name worker
kind get kubeconfig --name worker --internal > /tmp/worker.kubeconfig
kind get kubeconfig --name worker > /tmp/worker.client.kubeconfig

# restore your config
mv ~/.kube/config.save  ~/.kube/config

make e2e-apiserver-test
```

## Next steps

- Read our [code conventions](coding-conventions.md)
- Learn how to [Create a pull request](create-pull-request.md)
