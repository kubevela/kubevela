# Initializating Application

## `vela init`

To initialize and deploy an application with one service, run:

> If you only want to initialize without deploying the app, add `--render-only` flag

```bash
$ vela  init
Welcome to use KubeVela CLI! We're going to help you run applications through a couple of questions.

Environment: default, namespace: default

? What is the domain of your application service (optional):  example.com
? What is your email (optional, used to generate certification):
? What would you like to name your application (required):  testapp
? Choose the workload type for your application (required, e.g., webservice):  webservice
? What would you like to name this webservice (required):  testsvc
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

```bash
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

```bash
$ vela workloads
NAME      	DESCRIPTION
worker   	Backend worker without ports exposed
webservice	Long running service with network routes
```

Deploy the first service named `frontend` with `Web Service` type:

```bash
$ vela svc deploy frontend --app testapp -t webservice --image crccheck/hello-world
App testapp deployed
```

> TODO auto generate a random application name, so --app testapp becomes optional

Deploy the second service named `backend` with "Backend Worker" type:

```bash
$ vela svc deploy backend --app testapp2 -t worker --image crccheck/hello-world
App testapp2 deployed
```

```bash
$ vela ls
SERVICE 	APP     	TYPE	TRAITS	STATUS 	CREATED-TIME
frontend	testapp 	    	      	...
backend 	testapp 	    	      	...
```
