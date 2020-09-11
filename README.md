# KubeVela

The Open Application Platform based on Kubernetes and OAM.

## Get `vela` binary

```shell script
curl https://github.com/oam-dev/kubevela/releases/download/v0.0.1.2/vela-refs.tags.v0.0.1.2-darwin-amd64
chmod a+x vela-refs.tags.v0.0.1.2-darwin-amd64
mv vela-refs.tags.v0.0.1.2-darwin-amd64 /usr/local/bin/vela
```

## Install

```shell script
vela install
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

$ vela comp status app123
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

