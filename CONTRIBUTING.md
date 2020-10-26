# CONTRIBUTING

## About KubeVela

KubeVela project is initialized and maintained by the cloud native community since day 0 with [bootstrapping contributors from 8+ different organizations](https://github.com/oam-dev/kubevela/graphs/contributors). We intend for KubeVela to have a open governance since the very beginning and donate the project to neutral foundation as soon as it's released. 

This doc explains how to set up a development environment, so you can get started
contributing to `kubevela` or build a PoC (Proof of Concept). 

## Prerequisites

1. Golang version 1.13+
2. Kubernetes version v1.16+ with `~/.kube/config` configured.
3. ginkgo 1.14.0+ (just for [E2E test](https://github.com/oam-dev/kubevela/blob/master/DEVELOPMENT.md#e2e-test))
4. golangci-lint 1.31.0+, it will install automatically if you run `make`, you can [install it manually](https://golangci-lint.run/usage/install/#local-installation) if the installation is too slow.

## Build

* Clone this project

```shell script
git clone git@github.com:oam-dev/kubevela.git
```

* Build Vela CLI

```shell script
make
```

* Configure vela to PATH

after build, make will create `vela` binary to `bin/`, Set this path to PATH.

```shell script
export PATH=$PATH:/your/path/to/project/kubevela/bin
```

Then you can use `vela` command directly.

* Build Vela Core

```shell script
make manager
```

* Run Vela Core

Firstly make sure your cluster has CRDs.

```shell script
make core-install
```

Run locally:

```shell script
make core-run
```

This command will run controller locally, it will use your local KubeConfig which means you need to have a k8s cluster
locally. If you don't have a one, we suggest that you could setup up a cluster with [kind](https://kind.sigs.k8s.io/).

## Use

* Create environment
 
```shell script
vela env init myenv --namespace myenv --email my@email.com --domain kubevela.io 
```

* Create Component 

For example, use the following command to create and run an application.

```shell script
$ vela comp deploy mycomp -t webservice --image crccheck/hello-world --port 8000
Creating AppConfig appcomp
SUCCEED
```

* Add Trait

```shell script
$ vela route mycomp
Adding route for app abc
Succeeded!
```

* Check Status

```
$ vela comp status abc
Showing status of Component abc deployed in Environment t2
Component Status:
	Name: abc  PodSpecWorkload(type) UNKNOWN APIVersion standard.oam.dev/v1alpha1 Kind PodSpecWorkload workload is unknown for HealthScope
	Traits
	    └─Trait/route

Last Deployment:
	Created at: 2020-09-18 18:47:09 +0800 CST
	Updated at: 2020-09-18T18:47:16+08:00
```

* Delete App

```shell script
$ vela app ls
abc

$ vela app delete abc
Deleting Application "abc"
delete apps succeed abc from t2
```

## Tests

### Unit test

```shell script
make test
```

### E2E test

** Before e2e test start, make sure you have vela-core running.**

```shell script
make core-run
```

Start to test.

```
make e2e-test
```

## Make a pull request
Remember to write unit-test and e2e test before making a pull request.
