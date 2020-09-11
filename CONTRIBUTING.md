# CONTRIBUTING

This doc explains how to set up a development environment, so you can get started
contributing to `kubevela` or build a PoC (Proof of Concept). 

## Prerequisites

1. Golang version 1.12+
2. Kubernetes version v1.15+ with `~/.kube/config` configured.
3. OAM Kubernetes Runtime installed.
4. Kustomize version 3.8+
5. ginkgo 1.14.0+ (just for [E2E test](https://github.com/oam-dev/kubevela/blob/master/DEVELOPMENT.md#e2e-test))
6. golangci-lint 1.31.0+, it will install automatically if you run `make`, you can [install it manually](https://golangci-lint.run/usage/install/#local-installation) if the installation is too slow.

## Build
* Clone this project

```shell script
git clone git@github.com:oam-dev/kubevela.git
```

* Install Template CRD into your cluster

```shell script
make install
```

* Install template object 

```shell script
kubectl apply -f config/samples/
```

## Develop & Debug
If you change Template CRD, remember to rerun `make install`.

Use the following command to develop and debug.

```shell script
$ cd cmd/vela
$ go run main.go COMMAND [FLAG]
```

For example, use the following command to create and run an application.
```shell script
$ go run main.go run containerized app2057 nginx:1.9.4
Creating AppConfig app2057
SUCCEED

$ kubectl get oam
NAME                             WORKLOAD-KIND
component.core.oam.dev/app2057   ContainerizedWorkload

NAME                                     AGE
containerizedworkload.core.oam.dev/poc   53m

NAME                                            AGE
applicationconfiguration.core.oam.dev/app2057   69s

NAME                                                              DEFINITION-NAME
traitdefinition.core.oam.dev/simplerollouttraits.extend.oam.dev   simplerollouttraits.extend.oam.dev

NAME                                                                  DEFINITION-NAME
workloaddefinition.core.oam.dev/containerizedworkloads.core.oam.dev   containerizedworkloads.core.oam.dev
workloaddefinition.core.oam.dev/deployments.apps                      deployments.apps
workloaddefinition.core.oam.dev/statefulsets.apps                     statefulsets.apps
```

## E2E test
```
$ make e2e-test
Running Suite: Trait Suite
==========================
Random Seed: 1596559178
Will run 5 of 5 specs

Trait env init
  should print env initiation successful message
  kubevela/e2e/commonContext.go:14
Create env succeed, current env is default
•
------------------------------
Trait env set
  should show env set message
  kubevela/e2e/commonContext.go:40
Set env succeed, current env is default
•
------------------------------
Trait run
  should print successful creation information
  kubevela/e2e/commonContext.go:76
Creating AppConfig app-trait-basic
SUCCEED
•
------------------------------
Trait kubevela attach trait
  should print successful attached information
  kubevela/e2e/trait/trait_test.go:24
Applying trait for app
Succeeded!
•
------------------------------
Trait delete
  should print successful deletion information
  kubevela/e2e/commonContext.go:85
Deleting AppConfig "app-trait-basic"
DELETE SUCCEED
•
Ran 5 of 5 Specs in 9.717 seconds
SUCCESS! -- 5 Passed | 0 Failed | 0 Pending | 0 Skipped
PASS
```

## Make a pull request
Remember to write unit-test and e2e test before making a pull request.
