# Using Appfile for More Flexible Configuration

Appfile supports more flexible options than CLI/UI to configure appliation deployment on Vela.
A detailed design doc could be found [here](../../design/appfile-design.md)

In this tutorial, we will build and deploy an example NodeJS app under [examples/testapp/](https://github.com/oam-dev/kubevela/tree/master/examples/testapp).

## Prerequisites

- [docker](https://docs.docker.com/get-docker/) installed on the host
- [vela](../../install.md) installed
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) installed

## 1. Download test app code

git clone and go to the testapp directory:

```console
$ git clone https://github.com/oam-dev/kubevela.git
$ cd kubevela/examples/testapp
```

The example contains NodeJS app code, Dockerfile to build the app.

## 2. Deploy app in one command

[vela.yaml](../../../examples/testapp/vela.yaml) is an appfile format supported by Vela.

You need to change this line to the image that you can push to on your host:

> Or you may try the local kind cluster option which will be introduced in the following section.

```yaml
    image: oamdev/testapp:v1
```

Deploy vela.yaml:

```console
$ vela up
```

Now the app has been rendered and deployed.

## 3. Check rendered manifests and deployment

By default, Vela renders the final manifests in `.vela/deploy.yaml`:

```console
$ cat .vela/deploy.yaml
```

Check the status of the application deployment:

```console
$ vela app status testapp

$ vela svc status express-server
```

## [Optional] Configure "task" workload type

In above we deploy *webservice* workload. We can also deploy *task* workload via appfile.
Below is a simplified example from k8s doc:

```yaml
services:
  pi:
    image: perl 
    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

## [Optional] Local kind cluster testing without pushing image remotely

If you have local kind cluster running:

```console
$ kind get clusters
kind
```

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

[kind](https://kind.sigs.k8s.io/)

