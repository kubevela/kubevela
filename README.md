# KubeVela

The Open Application Platform based on Kubernetes and OAM.

:rotating_light: **Warning: The project is still under heavy development, its UI/UX is also for demo purpose, please don't look inside unless you know what you are doing** Please contact @wonderflow if you are interested in its full story or becoming one of the boostrap contributors/maintainers. :rotating_light:

## Install

### Prerequisites
- Kubernetes cluster running Kubernetes v1.15.0 or greater
- kubectl current context is configured for the target cluster install
  - ```kubectl config current-context```

### Get the Vela CLI

Download the `vela` binary from the [Releases page](https://github.com/oam-dev/kubevela/releases). Unpack the `vela` binary and add it to `$PATH` to get started.

```shell
sudo mv ./vela /usr/local/bin/vela
```

### Install Vela Core

```shell script
$ vela install
```

This command will install vela core controller into your K8s cluster, along with built-in workloads and traits.

## Demos

* Create ENV
 
```shell script
vela env init myenv --namespace myenv --email my@email.com --domain kubevela.io 
```

* Create Component 

For example, use the following command to create and run an application.

```shell script
$ vela comp run mycomp -t webservice --image crccheck/hello-world --port 8000
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
	Name: abc  Containerized(type) UNKNOWN APIVersion standard.oam.dev/v1alpha1 Kind Containerized workload is unknown for HealthScope
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

#### Auto-Completion

##### bash

```shell script
To load completions in your current shell session:
$ source <(vela completion bash)

To load completions for every new session, execute once:
Linux:
  $ vela completion bash > /etc/bash_completion.d/vela
MacOS:
  $ vela completion bash > /usr/local/etc/bash_completion.d/vela
```

##### zsh

```shell script
To load completions in your current shell session:
$ source <(vela completion zsh)

To load completions for every new session, execute once:
$ vela completion zsh > "${fpath[1]}/_vela"
```

### Clean your environment

```shell script
$ helm uninstall vela-core -n oam-system
release "vela-core" uninstalled
```

```shell script
$ kubectl delete crd workloaddefinitions.core.oam.dev traitdefinitions.core.oam.dev
customresourcedefinition.apiextensions.k8s.io "workloaddefinitions.core.oam.dev" deleted
customresourcedefinition.apiextensions.k8s.io "traitdefinitions.core.oam.dev" deleted
```

```shell script
$ rm -r ~/.vela
```

## CONTRIBUTING
Check out [CONTRIBUTING.md](./CONTRIBUTING.md) to see how to develop with KubeVela.

