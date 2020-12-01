# CONTRIBUTING Guide

## About KubeVela

KubeVela project is initialized and maintained by the cloud native community since day 0 with [bootstrapping contributors from 8+ different organizations](https://github.com/oam-dev/kubevela/graphs/contributors). We intend for KubeVela to have a open governance since the very beginning and donate the project to neutral foundation as soon as it's released. 

This doc explains how to set up a development environment, so you can get started
contributing to `kubevela` or build a PoC (Proof of Concept). 


## Development

### Prerequisites

1. Golang version 1.13+
2. Kubernetes version v1.16+ with `~/.kube/config` configured.
3. ginkgo 1.14.0+ (just for [E2E test](https://github.com/oam-dev/kubevela/blob/master/DEVELOPMENT.md#e2e-test))
4. golangci-lint 1.31.0+, it will install automatically if you run `make`, you can [install it manually](https://golangci-lint.run/usage/install/#local-installation) if the installation is too slow.

We also recommend you to learn about KubeVela's [design](docs/en/design.md) before dive into its code.

### Build

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

### Use

* Create environment
 
```shell script
vela env init myenv --namespace myenv --email my@email.com --domain kubevela.io 
```

* Create Component 

For example, use the following command to create and run an application.

```shell script
$ vela svc deploy mysvc -t webservice --image crccheck/hello-world --port 8000 -a abc
  App abc deployed
```

* Add Trait

```shell script
$ vela route abc
  Adding route for app mysvc
  ⠋ Deploying ...
  ✅ Application Deployed Successfully!
    - Name: mysvc
      Type: webservice
      HEALTHY Ready: 1/1
      Last Deployment:
        Created at: 2020-11-02 11:17:28 +0800 CST
        Updated at: 2020-11-02T11:21:23+08:00
      Routes:
        - route: Visiting URL: http://abc.kubevela.io	IP: 47.242.68.137
```

* Check Status

```
$ vela status abc
  About:
  
    Name:      	abc
    Namespace: 	default
    Created at:	2020-11-02 11:17:28.067738 +0800 CST
    Updated at:	2020-11-02 11:28:13.490986 +0800 CST
  
  Services:
  
    - Name: mysvc
      Type: webservice
      HEALTHY Ready: 1/1
      Last Deployment:
        Created at: 2020-11-02 11:17:28 +0800 CST
        Updated at: 2020-11-02T11:28:13+08:00
      Routes:
        - route: Visiting URL: http://abc.kubevela.io	IP: 47.242.68.137
```

* Delete App

```shell script
$ vela ls
  SERVICE       	APP      	TYPE	TRAITS	STATUS  	CREATED-TIME
  mysvc            	abc       	    	      	Deployed 	2020-11-02 11:17:28 +0800 CST

$ vela delete abc
  Deleting Application "abc"
  delete apps succeed abc from default
```

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

## Make a pull request
Remember to write unit-test and e2e test before making a pull request.

## Merge Regulations

Before merging, the pull request should obey the following rules:

- The commit title and message should be clear about what this PR does.
- All test CI should pass green.
- The `codecov/project` should pass. This means the coverage should not drop. See [Codecov commit status](https://docs.codecov.io/docs/commit-status#project-status).
