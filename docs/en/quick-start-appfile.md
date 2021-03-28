---
title:  Overview
---

To achieve best user experience for your platform, we recommend platform builders to create simple and user friendly UI for end users instead of exposing full platform level details to them. Some common practices include building GUI console, adopting DSL, or creating a user friendly command line tool.

As an proof-of-concept of building developer experience with KubeVela, we developed a client-side tool named `Appfile` as well. This tool enables developers to deploy any application with a single file and a single command: `vela up`.

Now let's walk through its experience.

## Step 1: Install

Make sure you have finished and verified the installation following [this guide](./install).

## Step 2: Deploy Your First Application

```bash
$ vela up -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/vela.yaml
Parsing vela.yaml ...
Loading templates ...

Rendering configs for service (testsvc)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward first-vela-app
             SSH: vela exec first-vela-app
         Logging: vela logs first-vela-app
      App status: vela status first-vela-app
  Service status: vela status first-vela-app --svc testsvc
```

Check the status until we see `Routes` are ready:
```bash
$ vela status first-vela-app
About:

  Name:       first-vela-app
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
    Traits:
      - ✅ ingress: Visiting URL: testsvc.example.com, IP: <your IP address>
```

**In [kind cluster setup](./install#kind)**, you can visit the service via localhost. In other setups, replace localhost with ingress address accordingly.

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

- Learn details about [`Appfile`](./developers/learn-appfile) and know how it works.
