# Deploying Application

## `vela app init`

The simplest way to deploy an application with KubeVela is using `$ vela app init` .

```console
$ vela app init
```

Check the application:

```console
$ vela show myapp
  About:
  
    Name:      	myapp
    Created at:	2020-11-02 11:39:04.626416 +0800 CST
    Updated at:	2020-11-02 11:39:04.627998 +0800 CST
  
  
  Environment:
  
    Namespace:	default
  
  Services:
  
    - Name:        	frontend
      WorkloadType:	webservice
      Arguments:
        port:         	8000
        image:        	crccheck/hello-world
        Traits:
          - route:
              domain:	frontend.example.com
              issuer:	oam-env-default
```

## Deploy Application Step by Step

You can also use KubeVela CLI to deploy a more complex micro-services application step by step, with detailed configurations.

Check the available workload types.

```console
$ vela workloads
TODO
```

Deploy the first service named `frontend` with `Web Service` type.

```console
$ vela svc deploy frontend -t webservice --image crccheck/hello-world --app myapp
App myapp deployed
```

> TODO auto generate a random application name, so --app myapp becomes optional

Deploy the second service named `backend` with "Backend Worker" type for the same application.

```console
$ vela svc deploy backend -t backendworker --image crccheck/hello-world --app myapp
Creating backend
SUCCEED
```

```console
$ vela ls
SERVICE       	APP      	TYPE	TRAITS	STATUS  	CREATED-TIME
frontend      	myapp    	    	      	Deployed	2020-11-02 11:39:05 +0800 CST
```
