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

After `vela install` you will have workloads and traits locally, and available to use by vela cli.

```shell script
$ vela workloads
NAME         	DEFINITION
backend      	containerizeds.standard.oam.dev
containerized	containerizedworkloads.core.oam.dev
task         	jobs
webservice   	containerizeds.standard.oam.dev
```

```shell script
$ vela traits
NAME        	DEFINITION                        	APPLIES TO
route       	routes.standard.oam.dev           	webservice
            	                                  	backend            	                                  
scale       	manualscalertraits.core.oam.dev   	webservice
            	                                  	backend
```

### Create ENV

Before working with your application, you should create an env for it.

```shell script
$ vela env init myenv --namespace myenv --email my@email.com --domain kubevela.io
ENV myenv CREATED, Namespace: myenv, Email: my@email.com.
```

It will create a namespace called myenv 

```shell script
$ kubectl get ns
NAME        STATUS   AGE
myenv       Active   40s
```

A namespace level issuer for certificate generation with email.
```shell script
$ kubectl get issuers.cert-manager.io -n myenv
  NAME            READY   AGE
  oam-env-myenv   True    40s
```

A env metadata in your local:

```shell script
$ cat ~/.vela/envs/myenv/config.json
  {"name":"myenv","namespace":"myenv","email":"my@email.com","domain":"kubevela.io","issuer":"oam-env-myenv"}
```


### Create Component 

Then let's create application, we will use our env created by default.

```shell script
$ vela comp run mycomp -t webservice --image crccheck/hello-world --port 8000 --app myapp
Creating AppConfig appcomp
SUCCEED
```

It will create component named `mycomp`.

```shell script
$ kubectl get components -n myenv
  NAME     WORKLOAD-KIND   AGE
  mycomp   Containerized   10s
```

And an AppConfig named myapp.

```shell script
$ kubectl get appconfig -n myenv
  NAME    AGE
  myapp   24s
```

Vela Core will work for AppConfig and create K8s deployment and service.

```shell script
$ kubectl get deployment -n myenv
  NAME     READY   UP-TO-DATE   AVAILABLE   AGE
  mycomp   1/1     1            1           38s
``` 

```shell script
$ kubectl get svc -n myenv
 NAME     TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)    AGE
 mycomp   ClusterIP   172.21.4.228   <none>        8080/TCP   49s
```

### Multiple Component

Creating a new component in the same application is easy, just use the `--app` flag.

```shell script
$ vela comp run db -t backend --image crccheck/hello-world --app myapp
Creating App myapp
SUCCEED
```

```shell script
$ vela comp ls
NAME  	APP  	WORKLOAD  	TRAITS	STATUS 	  CREATED-TIME
db    	myapp	backend   	      	Deployed	2020-09-18 22:42:04 +0800 CST
mycomp	myapp	webservice	      	Deployed	2020-09-18 22:42:04 +0800 CST
```

Now we can see the application deployed, let's add route trait for visiting.

### Add Trait

```shell script
$ vela route mycomp --app myapp
 Adding route for app mycomp
 Succeeded!
```

It will create route trait for this component.

```shell script
$ kubectl get routes.standard.oam.dev -n myenv
NAME                     AGE
mycomp-trait-5b576c4fc   18s
```

Controller of route trait which is part of vela core will create an ingress for it.

```shell script
$ kubectl get ingress -n myenv
NAME                        HOSTS                ADDRESS         PORTS     AGE
mycomp-trait-5b576c4fc      mycomp.kubevela.io   123.57.10.233   80, 443   73s
```

Please configure your domain pointing to the public address.

Then you will be able to visit it by `https://mycomp.kubevela.io`, `mTLS` is automatically enabled.


### Check Status


App level:

```shell script
$ vela app show myapp
  About:
  
    Name:      	myapp
    Created at:	2020-09-18 22:42:04.191171 +0800 CST
    Updated at:	2020-09-18 22:51:11.128997 +0800 CST
  
  
  Environment:
  
    Namespace:	myenv
  
  Components:
  
    Name  	Type      	Traits
    db    	backend
    mycomp	webservice	route
```

Component Level:

```shell script
$ vela comp show mycomp
 About:
 
   Name:        	mycomp
   WorkloadType:	webservice
   Application: 	myapp
 
 Environment:
 
   Namespace:	myenv
 
 Arguments:
 
   image:	crccheck/hello-world
   name: 	mycomp
   port: 	8000
 
 
 Traits:
 
   route:
     domain:	mycomp.kubevela.io
     issuer:	oam-env-myenv
     name:  	route
```

```
$ vela comp status mycomp
  Showing status of Component mycomp deployed in Environment myenv
  Component Status:
  	Name: mycomp  Containerized(type) UNKNOWN APIVersion standard.oam.dev/v1alpha1 Kind Containerized workload is unknown for HealthScope
  	Traits
  	    └─Trait/route
  
  Last Deployment:
  	Created at: 2020-09-18 22:42:04 +0800 CST
  	Updated at: 2020-09-18T22:51:11+08:00
```

### Delete App or Component

```shell script
$ vela app ls
myapp
```

```shell script
$ vela comp ls
NAME  	APP  	WORKLOAD  	TRAITS	STATUS 	CREATED-TIME
db    	myapp	backend   	      	Deployed	2020-09-18 22:42:04 +0800 CST
mycomp	myapp	webservice	route 	Deployed	2020-09-18 22:42:04 +0800 CST
```

```shell script
$ vela comp delete db
Deleting Component 'db' from Application 'db'
```

```shell script
$ vela comp ls
NAME  	APP  	WORKLOAD  	TRAITS	STATUS 	CREATED-TIME
mycomp	myapp	webservice	route 	Deployed	2020-09-18 22:42:04 +0800 CST
```

```shell script
$ vela app delete myapp
Deleting Application "myapp"
delete apps succeed myapp from myenv
```

## Dashboard

We also prepared a dashboard for you, but it's still in heavily development.

```shell script
$ vela dashboard
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

