# Quick Start

Welcome to KubeVela! In this guide, we'll walk you through how to install KubeVela, and deploy your first simple application.

## Step 1: Install

Make sure you have finished and verified the installation following [this guide](./install.md).

## Step 2: Deploy Your First Application

**vela init**

```bash
$ vela init --render-only
Welcome to use KubeVela CLI! Please describe your application.

Environment: default, namespace: default

? What is the domain of your application service (optional):  example.com
? What is your email (optional, used to generate certification):
? What would you like to name your application (required):  testapp
? Choose the workload type for your application (required, e.g., webservice):  webservice
? What would you like to name this webservice (required):  testsvc
? Which image would you like to use for your service (required): crccheck/hello-world
? Which port do you want customer traffic sent to (optional, default is 80): 8000

Deployment config is rendered and written to vela.yaml
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

**vela up**

```bash
$ vela up
Parsing vela.yaml ...
Loading templates ...

Rendering configs for service (testsvc)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc testsvc
```

Check the status until we see route trait ready:
```bash
$ vela status testapp
About:

  Name:       testapp
  Namespace:  default
  Created at: ...
  Updated at: ...

Services:

  - Name: testsvc
    Type: webservice
    HEALTHY Ready: 1/1
    Last Deployment:
      Created at: ...
      Updated at: ...
    Routes:
      - route:  Visiting URL: http://testsvc.example.com  IP: localhost
```

**In [kind cluster setup](./install.md#kind)**, you can visit the service via localhost. In other setups, replace localhost with ingress address accordingly.

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
**Voila!** You are all set to go.

## What's Next

Congratulations! You have just deployed an app using KubeVela. Here are some recommended next steps:

- Learn about the project's [motivation](./introduction.md) and [architecture](./design.md)
- Try out more [tutorials](./developers/config-enviroments.md)
- Join our community [Slack](https://cloud-native.slack.com/archives/C01BLQ3HTJA) and/or [Gitter](https://gitter.im/oam-dev/community)

Welcome onboard and sail Vela!
