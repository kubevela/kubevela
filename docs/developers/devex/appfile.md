# Using Appfile for More Flexible Configuration

Appfile supports more flexible options than CLI/UI to configure appliation deployment on Vela.
A detailed design doc could be found [here](../../design/appfile-design.md).

In this tutorial, we will build and deploy an example NodeJS app under [examples/testapp/](https://github.com/oam-dev/kubevela/tree/master/examples/testapp).

## Prerequisites

- [docker](https://docs.docker.com/get-docker/) installed on the host
- [vela](../../install.md) installed and configured

## 1. Download test app code

git clone and go to the testapp directory:

```console
$ git clone https://github.com/oam-dev/kubevela.git
$ cd kubevela/examples/testapp
```

The example contains NodeJS app code, Dockerfile to build the app.

## 2. Deploy app in one command

In the directory there is a [vela.yaml](../../../examples/testapp/vela.yaml) which follows Appfile format supported by Vela.
We are going to use it to build and deploy the app.

ATTENTION: change the image field in vela.yaml to something you can push to on your host:

> Or you may try the local testing option introduced in the following section.

```yaml
    image: oamdev/testapp:v1
```

Run the following command:

```console
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
writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
app has not been deployed, creating a new deployment...
✅ app has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela app status testapp
  Service status: vela svc status express-server
```


Check the status of the service:

```console
$ vela svc status express-server
Showing status of service(type: webservice) express-server deployed in Environment default
Service express-server Status:	 HEALTHY Ready: 1/1
	scaler: replica=1
...
```

### Alternative: Local testing without pushing image remotely

If you have local [kind](../../install.md#kind) cluster running, you may try the local push option without pushing image remotely.

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

```console
$ vela up
```

### [Optional] Check rendered manifests

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
    traits:
    - trait:
        apiVersion: core.oam.dev/v1alpha2
        kind: ManualScalerTrait
        ...
    - trait:
        apiVersion: standard.oam.dev/v1alpha1
        kind: Route
        ...
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

## 3. Add routing

Add routing config under `express-server`:

```yaml
servcies:
  express-server:
    ...

    route:
      domain: example.com
      rules:
        - path: /testapp(.*)
          rewriteTarget: /$1
```

Apply again:

```console
$ vela up
```

**In kind cluster**, we can visit the web service:

> If no in kind cluster, replace localhost with ingress address

```
$ curl -H "Host:example.com" http://localhost/testapp
Hello World
```

## 4. Add monitoring metrics

TODO

## [Optional] Configure "task" workload type

By now we have deployed a *webservice* workload. We can also add a *task* workload in appfile:

> Below is a simplified [k8s job example](https://kubernetes.io/docs/concepts/workloads/controllers/job/#running-an-example-job) using Vela:

```yaml
services:
  pi:
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]

  express-server:
    ...
```

Then deploy appfile again:

```console
$ vela up
```
