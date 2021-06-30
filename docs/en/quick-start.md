---
title:  Quick Start
---

Welcome to KubeVela! In this guide, we'll walk you through how to install KubeVela, and deploy your first simple application.

## Step 1: Install

Make sure you have finished and verified the installation following [this guide](./install).

## Step 2: Deploy Your First Application

```bash script
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/vela-app.yaml
```
```console
application.core.oam.dev/first-vela-app created
```

Above command will apply an application to KubeVela and let it distribute the application to proper runtime infrastructure.

Check the status until we see `status` is `running` and services are `healthy`:

```bash script
kubectl get application first-vela-app -o yaml
```
```console
apiVersion: core.oam.dev/v1beta1
kind: Application
...
status:
  ...
  services:
  - healthy: true
    name: express-server
    traits:
    - healthy: true
      message: 'Visiting URL: testsvc.example.com, IP: your ip address'
      type: ingress
  status: running
```

You can now directly visit the application (regardless of where it is running).

```bash script
curl -H "Host:testsvc.example.com" http://<your ip address>/
```
```console
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

Here are some recommended next steps:

- Learn KubeVela's [core concepts](./concepts)
- Learn more details about [`Application`](end-user/application) and what it can do for you.
- Learn how to attach [rollout plan](end-user/scopes/rollout-plan) to this application.
