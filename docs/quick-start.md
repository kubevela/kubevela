# Quick Start

Welcome to KubeVela! In this guide, we'll walk you through how to deploy a simple service application using KubeVela CLI/Appfile.

## Setup

Make sure you have finished and verified the installation following [this guide](./install.md).

## 1. Initialize Application

```console
$ vela init --render-only
Welcome to use KubeVela CLI! Please describe your application.

Environment: default, namespace: default

? Do you want to setup a domain for web service: example.com
? Provide an email for production certification:
? What would you like to name your application:  testapp
? Choose workload type for your service:  webservice
? What would you name this webservice:  testsvc
? specify app image: crccheck/hello-world
? specify port for container: 8000

Rendered and written deployment config to vela.yaml
```

In the current directory, you will find a generated `vela.yaml` file (i.e., an Appfile):

```yaml
createTime: ...
updateTime: ...

name: testapp
services:
  testsvc:
    type: webservice
    image: crccheck/hello-world
    port: 8000
    route:
      domain: testsvc.example.com
```

## 2. Deploy Application

```console
$ vela up
Parsing vela.yaml ...
Loading templates ...

Rendering configs for service (testsvc)...
writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
app existed, updating existing deployment...
✅ app has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc testsvc
```

Check the status until we see route trait ready:
```console
$ vela status testapp
About:

  Name:      	testapp
  Namespace: 	default
  Created at:	...
  Updated at:	...

Services:

  - Name: testsvc
    Type: webservice
    HEALTHY Ready: 1/1
    Last Deployment:
      Created at: ...
      Updated at: ...
    Routes:
      - route: 	Visiting URL: http://testsvc.example.com	IP: localhost
```

**In [kind cluster setup](../../install.md#kind)**, you can visit the service via localhost. In other setups, replace localhost with ingress address accordingly.

```
$ curl -H "Host:testsvc.example.com" http://localhost/
<xmp>
Hello World


                                       ##         .
                                 ## ## ##        ==
                              ## ## ## ## ##    ===
                           /""""""""""""""""\___/ ===
                      ~~~ {~~ ~~~~ ~~~ ~~~~ ~~ ~ /  ===- ~~~
                           \______ o          _,/
                            \      \       _,'
                             `'--.._\..--''
</xmp>
```

## What's Next

Congratulations! You have just deployed an app using KubeVela. Here are some recommended next steps:

- Learn about the project's [motivation](./introduction.md) and [architecture](./design.md)
- Try out more [tutorials](./README.md)
- Join our community [Slack](https://cloud-native.slack.com/archives/C01BLQ3HTJA) and/or [Gitter](https://gitter.im/oam-dev/community)

Welcome onboard and sail Vela!
