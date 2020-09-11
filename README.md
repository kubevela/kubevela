# KubeVela

The Open Application Platform based on Kubernetes and OAM.

## Install `vela` binary

```shell script
curl https://github.com/oam-dev/kubevela/releases/download/v0.0.1.2/vela-refs.tags.v0.0.1.2-darwin-amd64
chmod a+x vela-refs.tags.v0.0.1.2-darwin-amd64
mv vela-refs.tags.v0.0.1.2-darwin-amd64 /usr/local/bin/vela
```

## Vela commands

```shell script
$ vela
✈️  A Micro App Platform for Kubernetes.

Usage:
  vela [flags]
  vela [command]

Available Commands:

  Getting Started:

    env             	Manage application environments
      delete        	  Delete environment
      init <envName>	  Create environment and switch to it
      ls            	  List all environments
      switch        	  switch to another environment
    version         	Prints out build version information

  Applications:

    app                                   	Manage applications with ls, show, delete, run
      delete <APPLICATION_NAME>           	  Delete Applications
      ls                                  	  List applications with workloads, traits, status and created time
      run <APPLICATION_BUNDLE_NAME> [args]	  Run a bundle of OAM Applications
      show <APPLICATION-NAME>             	  get details of your app, including its workload and trait
      status <APPLICATION-NAME>           	  get status of an application, including its workload and trait
    comp                                  	Manage Components
      delete <ComponentName>              	  Delete Component From Application
      ls                                  	  List applications with workloads, traits, status and created time
      run [args]                          	  Init and Run workloads
      show <COMPONENT-NAME>               	  get component detail, including arguments of workload and trait

  Traits:

    rollout <appname> [args]	Attach rollout trait to an app
    route <appname> [args]  	Attach route trait to an app
    scale <appname> [args]  	Attach scale trait to an app

    Want more? < install more capabilities by `vela cap` >

  Others:

    cap                  	Capability Management with config, list, add, remove capabilities
      add <center>/<name>	  Add capability into cluster
      center <command>   	  Manage Capability Center with config, sync, list
      ls [centerName]    	  List all capabilities in center
      remove <name>      	  Remove capability from cluster

  System:

    completion < bash | zsh >	Output shell completion code for the specified shell (bash or zsh...
      bash                   	  Generate the autocompletion script for Vela for the bash shell....
      zsh                    	  Generate the autocompletion script for Vela for the zsh shell.

                             	T...
    dashboard                	Setup API Server and launch Dashboard
    system                   	system management utilities
      info                   	  show vela client and cluster version
      init                   	  Install OAM runtime and vela builtin capabilities.
      update                 	  Refresh and sync definition files from cluster
```

#### env

```
$ vela env init test --namespace test
Create env succeed, current env is test

$ vela env ls
NAME           	CURRENT	NAMESPACE
default             	default
test    	 	*       test

$ vela env switch default
Switch env succeed, current env is default

$ vela env delete test
test deleted

$ vela env delete default
Error: you can't delete current using default
```

#### workload run
```shell script
$ vela comp run -t deployment app123 -p 80 --image nginx:1.9.4
Creating AppConfig app123
SUCCEED
```

#### app
```
$ vela app ls
app123
poc08032042
poc1039

$ vela comp ls
NAME 	APP  	WORKLOAD  	TRAITS     	STATUS  	CREATED-TIME
ccc  	ccc  	deployment	           	Deployed	2020-08-27 10:56:41 +0800 CST
com1 	com1 	          	           	Deployed	2020-08-26 16:45:50 +0800 CST
com2 	com1 	          	           	Deployed	2020-08-26 16:45:50 +0800 CST
myapp	myapp	          	route,scale	Deployed	2020-08-19 15:11:17 +0800 CST

$ vela app delete app123
Deleting AppConfig "app123"
DELETE SUCCEED
```

#### WorkloadDefinitions/TraitDefinitions
```shell script
$ vela traits
NAME                              	ALIAS	DEFINITION                        	APPLIES TO                                                  	STATUS
manualscalertraits.core.oam.dev   	     	manualscalertraits.core.oam.dev   	core.oam.dev/v1alpha2.ContainerizedWorkload                 	-
simplerollouttraits.extend.oam.dev	     	simplerollouttraits.extend.oam.dev	core.oam.dev/v1alpha2.ContainerizedWorkload, deployments....	-

$ vela workloads
NAME                               	SHORT	DEFINITION
containerizedworkloads.core.oam.dev	     	containerizedworkloads.core.oam.dev
deployments.apps                   	     	deployments.apps
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
$ helm uninstall core-runtime -n oam-system
release "core-runtime" uninstalled
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

