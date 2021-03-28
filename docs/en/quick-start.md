---
title:  Quick Start
---

Welcome to KubeVela! In this guide, we'll walk you through how to install KubeVela, and deploy your first simple application.

## Step 1: Install

Make sure you have finished and verified the installation following [this guide](./install).

## Step 2: Deploy Your First Application

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/vela-app.yaml
application.core.oam.dev/first-vela-app created
```

Check the status until we see `status` is `running` and services are `healthy`:

```bash
$  kubectl get application first-vela-app -o yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  generation: 1
  name: first-vela-app
  ...
  namespace: default
spec:
  components:
  - name: express-server
    type: webservice
    properties:
      image: crccheck/hello-world
      port: 8000
    traits:
    - type: ingress
      properties:
        domain: testsvc.example.com
        http:
          /: 8000
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

Under the neath, the K8s resources was created:

```bash
$ kubectl get deployment
NAME                READY   UP-TO-DATE   AVAILABLE   AGE
express-server-v1   1/1     1            1           8m
$ kubectl get svc
NAME             TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
express-server   ClusterIP   172.21.11.152   <none>        8000/TCP   7m43s
kubernetes       ClusterIP   172.21.0.1      <none>        443/TCP    116d
$ kubectl get ingress
NAME             CLASS    HOSTS                 ADDRESS          PORTS   AGE
express-server   <none>   testsvc.example.com   <your ip address>   80      7m47s
```

If your cluster has a working ingress, you can visit the service.

```
$ curl -H "Host:testsvc.example.com" http://<your ip address>/
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

- Learn KubeVela starting from its [core concepts](./concepts)
- Learn more details about [`Application`](./application) and understand how it works.
- Join `#kubevela` channel in CNCF [Slack](https://cloud-native.slack.com) and/or [Gitter](https://gitter.im/oam-dev/community)

Welcome onboard and sail Vela!