# CONTRIBUTING Guide

## About KubeVela

KubeVela project is initialized and maintained by the cloud native community since day 0 with [bootstrapping contributors from 8+ different organizations](https://github.com/oam-dev/kubevela/graphs/contributors).
We intend for KubeVela to have an open governance since the very beginning and donate the project to neutral foundation as soon as it's released. 

This doc explains how to set up a development environment, so you can get started
contributing to `kubevela` or build a PoC (Proof of Concept). 


## Development

### Prerequisites

1. Golang version 1.16+
2. Kubernetes version v1.16+ with `~/.kube/config` configured.
3. ginkgo 1.14.0+ (just for [E2E test](./CONTRIBUTING.md#e2e-test))
4. golangci-lint 1.31.0+, it will install automatically if you run `make`, you can [install it manually](https://golangci-lint.run/usage/install/#local-installation) if the installation is too slow.

We also recommend you to learn about KubeVela's [design](https://kubevela.io/docs/concepts) before dive into its code.

### Build

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

Run locally:

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

### Use

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

### Contribute Docs

Please read [the documentation](https://github.com/oam-dev/kubevela/tree/master/docs/README.md) before contributing to the docs.

- Build docs

```shell script
make docs-build
```

- Local development and preview

```shell script
make docs-start
```

## Make a pull request

Remember to write unit-test and e2e-test after you have finished your code.
 
Run following checks before making a pull request.

```shell script
make reviewable
```

The command will do some lint checks and clean code.

After that, check in all changes and send a pull request.

## Merge Regulations

Before merging, the pull request should obey the following rules:

- The commit title and message should be clear about what this PR does.
- All test CI should pass green.
- The `codecov/project` should pass. This means the coverage should not drop. See [Codecov commit status](https://docs.codecov.io/docs/commit-status#project-status).
