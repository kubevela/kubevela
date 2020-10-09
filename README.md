# KubeVela

The Open Application Platform based on Kubernetes and Open Application Model (OAM).

## Project Status

:rotating_light: **Warning: this project is still a work in progress with lots of rough edges, please don't look inside unless you know what you are doing.**

KubeVela project is initialized and maintained by the cloud native community since day 0 with [bootstrapping contributors from 8+ different organizations](https://github.com/oam-dev/kubevela/graphs/contributors). We intend for KubeVela to have a open governance since the very beginning and donate the project to neutral foundation as soon as it's released. 

## Purpose and Goal

- For developers and operators
  - KubeVela, as an out-of-box Cloud Native Application Management Platform, provides numerous workloads and operation tooling for application defining, deployment, scaling, traffic, rollout, routing, monitoring, logging, alerting, CI/CD and so on.
- For platform builders
  - KubeVela, as a highly extensible PaaS/Serverless Core, provides pluggable capabilities, an elegant way to integrate any workloads and operational capabilities (i.e. traits).

## Design and Architecture

Read more about [KubeVela's high level design and architecture](DESIGN.md).

## Demo Instructions

See the demo instructions below get a sense of what we've accomplished and are working on.

## Install

### Prerequisites
- Kubernetes cluster running Kubernetes v1.15.0 or greater
- kubectl current context is configured for the target cluster install
  - ```kubectl config current-context```

### Get the Vela CLI

Download the `vela` binary from the [releases page](https://github.com/oam-dev/kubevela/releases). Unpack the `vela` binary and add it to `$PATH` to get started.

```shell
sudo mv ./vela /usr/local/bin/vela
```

### Install Vela Core

```console
$ vela install
```
This command will install vela core controller into your K8s cluster, along with built-in workloads and traits.

## Using KubeVela

After `vela install` you will see available workloads and traits.

```console
$ vela workloads
NAME         	DEFINITION
backend      	podspecworkloads.standard.oam.dev
task         	jobs.batch.k8s.io
webservice   	podspecworkloads.standard.oam.dev
```

```console
$ vela traits
NAME        	DEFINITION                        	APPLIES TO
route       	routes.standard.oam.dev           	webservice,backend            	                                  
scale       	manualscalertraits.core.oam.dev   	webservice,backend
```

### Create environment

Before working with your application, you should prepare an deploy environment for it (e.g. test, staging, prod etc).

```console
$ vela env init demo --namespace demo --email my@email.com --domain kubevela.io
ENVIROMENT demo CREATED, Namespace: demo, Email: my@email.com.
```

Vela will create a Kubernetes namespace called `demo` , with namespace level issuer for certificate generation using the email you provided.

You could check the environment metadata in your local:

```console
$ cat ~/.vela/envs/demo/config.json
  {"name":"demo","namespace":"demo","email":"my@email.com","domain":"kubevela.io","issuer":"oam-env-demo"}
```


### Create simple component 

Then let's create application, we will use the `demo` environment.

```console
$ vela comp deploy mycomp -t webservice --image crccheck/hello-world --port 8000 --app myapp
Creating AppConfig appcomp
SUCCEED
```

### Create micro-services application

Vela supports micro-services application by default thanks to Open Application Model.

```console
$ vela comp deploy db -t backend --image crccheck/hello-world --app myapp
Creating App myapp
SUCCEED
```

```console
$ vela comp ls
NAME  	APP  	WORKLOAD  	TRAITS	STATUS 	  CREATED-TIME
db    	myapp	backend   	      	Deployed	2020-09-18 22:42:04 +0800 CST
mycomp	myapp	webservice	      	Deployed	2020-09-18 22:42:04 +0800 CST
```

#### Under the hood

In Kubernetes, vela creates an OAM application configuration named `myapp` to manage all related components.

```console
$ kubectl get appconfig -n demo
  NAME    AGE
  myapp   24s
```

```console
$ kubectl get components -n demo
  NAME     AGE
  mycomp   24s
  db       10s
```

Vela Core is responsible for managing the underlying Kubernetes resources linked with the components and application configuration above.

```console
$ kubectl get deployment -n demo
  NAME     READY   UP-TO-DATE   AVAILABLE   AGE
  mycomp   1/1     1            1           38s
  db       1/1     1            1           20s
``` 

```console
$ kubectl get svc -n demo
 NAME     TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)    AGE
 mycomp   ClusterIP   172.21.4.228   <none>        8080/TCP   49s
```

### Manage operational configurations of the application

Vela leverages OAM trait system to manage operational configurations such as `scale`, `route`, `canary`, `autocale`etc in application centric approach.

Let's take `route` as example.

### `route`

```console
$ vela route mycomp --app myapp
 Adding route for app mycomp
 Succeeded!
```

For now you have to check the public address manually (this will be fixed soon so `vela route` will return visiting URL as result):

```console
$ kubectl get ingress -n demo
NAME                        HOSTS                ADDRESS         PORTS     AGE
mycomp-trait-5b576c4fc      mycomp.kubevela.io   123.57.10.233   80, 443   73s
```

And after you configure the `kubevela.io` domain pointing to the public address above.

Your application will be reached by `https://mycomp.kubevela.io` with `mTLS` automatically enabled.

### Under the hood

Vela will manage the underlying Kubernetes resource which implements the `route` trait.

```console
$ kubectl get routes.standard.oam.dev -n demo
NAME                     AGE
mycomp-trait-5b576c4fc   18s
```

`routes.standard.oam.dev` is a CRD controller which will manage ingress, domain, certificate etc for your application.

### Check status


Check the application:

```console
$ vela app show myapp
  About:
  
    Name:      	myapp
    Created at:	2020-09-18 22:42:04.191171 +0800 CST
    Updated at:	2020-09-18 22:51:11.128997 +0800 CST
  
  
  Environment:
  
    Namespace:	demo
  
  Components:
  
    Name  	Type      	Traits
    db    	backend
    mycomp	webservice	route
```

Check specific component:

```console
$ vela comp show mycomp
 About:
 
   Name:        	mycomp
   WorkloadType:	webservice
   Application: 	myapp
 
 Environment:
 
   Namespace:	demo
 
 Arguments:
 
   image:	crccheck/hello-world
   name: 	mycomp
   port: 	8000
 
 
 Traits:
 
   route:
     domain:	mycomp.kubevela.io
     issuer:	oam-env-demo
     name:  	route
```

```
$ vela comp status mycomp
  Showing status of Component mycomp deployed in Environment demo
  Component Status:
  	Name: mycomp  PodSpecWorkload(type) UNKNOWN APIVersion standard.oam.dev/v1alpha1 Kind PodSpecWorkload workload is unknown for HealthScope
  	Traits
  	    └─Trait/route
  
  Last Deployment:
  	Created at: 2020-09-18 22:42:04 +0800 CST
  	Updated at: 2020-09-18T22:51:11+08:00
```

### Delete application or component

```console
$ vela app ls
myapp
```

```console
$ vela comp ls
NAME  	APP  	WORKLOAD  	TRAITS	STATUS 	CREATED-TIME
db    	myapp	backend   	      	Deployed	2020-09-18 22:42:04 +0800 CST
mycomp	myapp	webservice	route 	Deployed	2020-09-18 22:42:04 +0800 CST
```

```console
$ vela comp delete db
Deleting Component 'db' from Application 'db'
```

```console
$ vela comp ls
NAME  	APP  	WORKLOAD  	TRAITS	STATUS 	CREATED-TIME
mycomp	myapp	webservice	route 	Deployed	2020-09-18 22:42:04 +0800 CST
```

```console
$ vela app delete myapp
Deleting Application "myapp"
delete apps succeed myapp from demo
```

## Dashboard

Vela has a simple client side dashboard for you to interact with (note it's still under development). The functionality is equivalent to the vela cli.

```console
$ vela dashboard
```

#### Auto-completion

##### bash

```console
To load completions in your current shell session:
$ source <(vela completion bash)

To load completions for every new session, execute once:
Linux:
  $ vela completion bash > /etc/bash_completion.d/vela
MacOS:
  $ vela completion bash > /usr/local/etc/bash_completion.d/vela
```

##### zsh

```console
To load completions in your current shell session:
$ source <(vela completion zsh)

To load completions for every new session, execute once:
$ vela completion zsh > "${fpath[1]}/_vela"
```

### Clean your environment

```console
$ helm uninstall kubevela -n vela-system
release "kubevela" uninstalled
```

```console
$ kubectl delete crd workloaddefinitions.core.oam.dev traitdefinitions.core.oam.dev  scopedefinitions.core.oam.dev
customresourcedefinition.apiextensions.k8s.io "workloaddefinitions.core.oam.dev" deleted
customresourcedefinition.apiextensions.k8s.io "traitdefinitions.core.oam.dev" deleted
```

```console
$ rm -r ~/.vela
```

## Contributing
Check out [CONTRIBUTING.md](./CONTRIBUTING.md) to see how to develop with KubeVela.

## Code of Conduct
This project has adopted the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md). See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for further details.

