# KubeVela

The Open Application Platform based on Kubernetes and OAM.

:rotating_light: **Warning: The project is still under heavy development, its UI/UX is also for demo purpose, please don't look inside unless you know what you are doing** Please contact @wonderflow if you are interested in its full story or becoming one of the boostrap contributors/maintainers. :rotating_light:

## Install

### Prerequisites
- Kubernetes cluster running Kubernetes v1.15.0 or greater
- kubectl current context is configured for the target cluster install
  - ```kubectl config current-context```

### Get the Vela CLI

Download the `vela` binary from the [Releases page](https://github.com/oam-dev/kubevela/releases). Change file mod and add it to `$PATH` to get started.

For exmaple:
```shell
chmod a+x vela-v0.0.2-darwin-amd64
sudo mv ./vela-v0.0.2-darwin-amd64 /usr/local/bin/vela
```

### Install Vela Core

```shell script
$ vela install
- Installing Vela Core:
- Installing builtin capabilities:
Successful applied 4 kinds of Workloads and Traits: deployments.apps,containerizedworkloads.core.oam.dev,manualscalertraits.core.oam.dev,simplerollouttraits.extend.oam.dev.
syncing workload definitions from cluster...
[WARN]handle template task: #Template.metadata.name: reference "task" not found
get 5 workload definitions from cluster, syncing...5 workload definitions successfully synced
syncing trait definitions from cluster...
[WARN]handle template metricstraits.standard.oam.dev: #Template.metadata.name: reference "metricstraits" not found
get 2 trait definitions from cluster, syncing...2 trait definitions successfully synced
- Finished.
```

## Demos

#### Check workloads

```
$ vela workloads
  NAME         	DEFINITION
  backend      	containerizeds.standard.oam.dev
  task         	jobs
  webservice   	containerizeds.standard.oam.dev
```

#### workload run
```shell script
$ vela comp run -t webservice app123 -p 80 --image nginx:1.9.4
Creating AppConfig app123
SUCCEED

$ vela comp status app123

$ vela comp ls
NAME 	APP  	WORKLOAD  	TRAITS     	STATUS  	CREATED-TIME
app123  app123  deployment	           	Deployed	2020-08-27 10:56:41 +0800 CST
```

#### app

```
$ vela app ls
app123

$ vela app delete app123
Deleting AppConfig "app123"
DELETE SUCCEED
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

