# Deploying Application

## `vela app init`

The simplest way to deploy an application with KubeVela is using `$ vela app init` .

```console
$ vela app init
```

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
  
    Name  	    Type      	Traits
    frontend	webservice	route
```

Check the deployed service:

```console
$ vela svc show mycomp
 About:
 
   Name:        	frontend
   WorkloadType:	webservice
   Application: 	myapp
 
 Environment:
 
   Namespace:	demo
 
 Arguments:
 
   image:	crccheck/hello-world
   name: 	frontend
   port: 	8000
 
 
 Traits:
 
   route:
     domain:	frontend.kubevela.io
     issuer:	oam-env-demo
     name:  	route
```

## Step by Step

You can also use KubeVela CLI to deploy application step by step, with more detailed configurations.

Check the available workload types.

```console
$ vela workloads
TODO
```

Deploy the first service named `frontend` with `Web Service` type.

```console
$ vela svc deploy frontend -t webservice --image crccheck/hello-world --app myapp
Creating frontend ...
SUCCEED
```

> TODO auto generate a random application name, so --app myapp becomes optional

Deploy the second service named `backend` with "Backend Worker" type for the same application.

```console
$ vela svc deploy backend -t backendworker --image crccheck/hello-world --app myapp
Creating backend
SUCCEED
```

```console
$ vela svc ls
NAME  	    APP  	WORKLOAD  	  TRAITS	STATUS 	    CREATED-TIME
backend    	myapp	backendworker   	    Deployed	2020-09-18 22:42:04 +0800 CST
frontend	myapp	webservice	      	    Deployed	2020-09-18 22:42:04 +0800 CST
```
