# Learning Appfile Step by Step

Appfile is the main user interface to configure application deployment on Vela.

In this tutorial, we will build and deploy an example NodeJS app under [examples/testapp/](https://github.com/oam-dev/kubevela/tree/master/docs/examples/testapp).

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) installed on the host
- [KubeVela](../install.md) installed and configured

## 1. Download test app code

git clone and go to the testapp directory:

```bash
$ git clone https://github.com/oam-dev/kubevela.git
$ cd kubevela/docs/examples/testapp
```

The example contains NodeJS app code, Dockerfile to build the app.

## 2. Deploy app in one command

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

### Alternative: Local testing without pushing image remotely

If you have local [kind](../install.md) cluster running, you may try the local push option. No remote container registry is needed in this case.

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

## [Optional] Configure another workload type

By now we have deployed a *[Web Service](references/workload-types/webservice.md)*, which is the default workload type in KubeVela. We can also add another service of *[Task](references/workload-types/task.md)* type in the same app:

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

> Interested in the more details of Appfile? [Learn Full Schema of Appfile](references/devex/appfile.md)

## What's Next?

Congratulations! You have just deployed an app using Vela.

Some tips that you can have more play with your app:
- [Check Application Logs](./check-logs.md)
- [Execute Commands in Application Container](./exec-cmd.md)
- [Access Application via Route](./port-forward.md)

