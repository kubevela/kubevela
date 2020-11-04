# Initializating Application

## `vela init`

To initialize and deploy an application with one service, run:

> If you only want to initialize without deploying the app, add `--render-only` flag

```console
$ vela  init
Welcome to use KubeVela CLI! We're going to help you run applications through a couple of questions.

Environment: default, namespace: default

? Do you want to setup a domain for web service:
? Provide an email for production certification:
? What would you like to name your application:  testapp
? Choose workload type for your service:  webservice
? What would you name this webservice:  testsvc
? specify app image crccheck/hello-world
? specify port for container 8000
...
✅ Application Deployed Successfully!
  - Name: testsvc
    Type: webservice
    HEALTHY Ready: 1/1
    Routes:

    Last Deployment:
      Created at: ...
      Updated at: ...
```

Check the application:

```console
$ vela show testapp
About:

  Name:      	testapp
  Created at:	...
  Updated at:	...


Environment:

  Namespace:	default

Services:

  - Name:        	testsvc
    WorkloadType:	webservice
    Arguments:
      image:        	crccheck/hello-world
      port:         	8000
      Traits:
```

## Deploy Multiple Services

You can also use KubeVela CLI to deploy multiple services for an application.

Check the available workload types.

```console
$ vela workloads
NAME      	DESCRIPTION
worker   	Backend worker without ports exposed
webservice	Long running service with network routes
```

Deploy the first service named `frontend` with `Web Service` type:

```console
$ vela svc deploy frontend --app testapp -t webservice --image crccheck/hello-world
App testapp deployed
```

> TODO auto generate a random application name, so --app testapp becomes optional

Deploy the second service named `backend` with "Backend Worker" type:

```console
$ vela svc deploy backend --app testapp2 -t worker --image crccheck/hello-world
App testapp2 deployed
```

```console
$ vela ls
SERVICE 	APP     	TYPE	TRAITS	STATUS 	CREATED-TIME
frontend	testapp 	    	      	...
backend 	testapp 	    	      	...
```
