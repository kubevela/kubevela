---
title:  Learning Appfile
---

A sample `Appfile` is as below:

```yaml
name: testapp

services:
  frontend: # 1st service

    image: oamdev/testapp:v1
    build:
      docker:
        file: Dockerfile
        context: .

    cmd: ["node", "server.js"]
    port: 8080

    route: # trait
      domain: example.com
      rules:
        - path: /testapp
          rewriteTarget: /

  backend: # 2nd service
    type: task # workload type
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

Under the hood, `Appfile` will build the image from source code, and then generate `Application` resource with the image name.

## Schema

> Before learning about Appfile's detailed schema, we recommend you to get familiar with [core concepts](../concepts) in KubeVela.


```yaml
name: _app-name_

services:
  _service-name_:
    # If `build` section exists, this field will be used as the name to build image. Otherwise, KubeVela will try to pull the image with given name directly.
    image: oamdev/testapp:v1

    build:
      docker:
        file: _Dockerfile_path_ # relative path is supported, e.g. "./Dockerfile"
        context: _build_context_path_ # relative path is supported, e.g. "."

      push:
        local: kind # optionally push to local KinD cluster instead of remote registry

    type: webservice (default) | worker | task

    # detailed configurations of workload
    ... properties of the specified workload  ...

    _trait_1_:
      # properties of trait 1

    _trait_2_:
      # properties of trait 2

    ... more traits and their properties ...
  
  _another_service_name_: # more services can be defined
    ...
  
```

> To learn about how to set the properties of specific workload type or trait, please check the [reference documentation guide](./check-ref-doc).

## Example Workflow

In the following workflow, we will build and deploy an example NodeJS app under [examples/testapp/](https://github.com/oam-dev/kubevela/tree/master/docs/examples/testapp).

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) installed on the host
- [KubeVela](../install) installed and configured

### 1. Download test app code

git clone and go to the testapp directory:

```bash
$ git clone https://github.com/oam-dev/kubevela.git
$ cd kubevela/docs/examples/testapp
```

The example contains NodeJS app code, Dockerfile to build the app.

### 2. Deploy app in one command

In the directory there is a [vela.yaml](https://github.com/oam-dev/kubevela/tree/master/docs/examples/testapp/vela.yaml) which follows Appfile format supported by Vela.
We are going to use it to build and deploy the app.

> NOTE: please change `oamdev` to your own registry account so you can push. Or, you could try the alternative approach in `Local testing without pushing image remotely` section.

```yaml
    image: oamdev/testapp:v1 # change this to your image
```

Run the following command:

```bash
$ vela up
Parsing vela.yaml ...
Loading templates ...

Building service (express-server)...
Sending build context to Docker daemon  71.68kB
Step 1/10 : FROM mhart/alpine-node:12
 ---> 9d88359808c3
...

pushing image (oamdev/testapp:v1)...
...

Rendering configs for service (express-server)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc express-server
```


Check the status of the service:

```bash
$ vela status testapp
  About:
  
    Name:       testapp
    Namespace:  default
    Created at: 2020-11-02 11:08:32.138484 +0800 CST
    Updated at: 2020-11-02 11:08:32.138485 +0800 CST
  
  Services:
  
    - Name: express-server
      Type: webservice
      HEALTHY Ready: 1/1
      Last Deployment:
        Created at: 2020-11-02 11:08:33 +0800 CST
        Updated at: 2020-11-02T11:08:32+08:00
      Routes:

```

#### Alternative: Local testing without pushing image remotely

If you have local [kind](../install) cluster running, you may try the local push option. No remote container registry is needed in this case.

Add local option to `build`:

```yaml
    build:
      # push image into local kind cluster without remote transfer
      push:
        local: kind

      docker:
        file: Dockerfile
        context: .
```

Then deploy the app to kind:

```bash
$ vela up
```

<details><summary>(Advanced) Check rendered manifests</summary>

By default, Vela renders the final manifests in `.vela/deploy.yaml`:

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: testapp
  namespace: default
spec:
  components:
  - componentName: express-server
---
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: express-server
  namespace: default
spec:
  workload:
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: express-server
    ...
---
apiVersion: core.oam.dev/v1alpha2
kind: HealthScope
metadata:
  name: testapp-default-health
  namespace: default
spec:
  ...
```
</details>

### [Optional] Configure another workload type

By now we have deployed a *[Web Service](references/component-types/webservice)*, which is the default workload type in KubeVela. We can also add another service of *[Task](references/component-types/task)* type in the same app:

```yaml
services:
  pi:
    type: task
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]

  express-server:
    ...
```

Then deploy Appfile again to update the application:

```bash
$ vela up
```

Congratulations! You have just deployed an app using `Appfile`.

## What's Next?

Play more with your app:
- [Check Application Logs](./check-logs)
- [Execute Commands in Application Container](./exec-cmd)
- [Access Application via Route](./port-forward)

