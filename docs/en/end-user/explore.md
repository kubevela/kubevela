---
title: Explore Applications
---

We will introduce how to explore application related resources in this section.

## List Application

```shell
$ kubectl get application
NAME        COMPONENT   TYPE         PHASE     HEALTHY   STATUS   AGE
app-basic   app-basic   webservice   running   true               12d
website     frontend    webservice   running   true               4m54s
```

You can also use the short name `kubectl get app`.

### View Application Details

```shell
$ kubectl get app website -o yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  generation: 1
  name: website
  namespace: default
spec:
  components:
  - name: frontend
    properties:
      image: nginx
    traits:
    - properties:
        cpuPercent: 60
        max: 10
        min: 1
      type: cpuscaler
    - properties:
        image: fluentd
        name: sidecar-test
      type: sidecar
    type: webservice
  - name: backend
    properties:
      cmd:
      - sleep
      - "1000"
      image: busybox
    type: worker
status:
  ...
  latestRevision:
    name: website-v1
    revision: 1
    revisionHash: e9e062e2cddfe5fb
  services:
  - healthy: true
    name: frontend
    traits:
    - healthy: true
      type: cpuscaler
    - healthy: true
      type: sidecar
  - healthy: true
    name: backend
  status: running
```

Here are some highlight information that you need to know:

1. `status.latestRevision` declares current revision of this application.
2. `status.services` declares the component created by this application and the healthy state.
3. `status.status` declares the global state of this application. 

### List Application Revisions

> Note: we currently don't support list by label selector until https://github.com/oam-dev/kubevela/issues/1476

When we update an application, if there's any difference on spec, KubeVela will create a new revision.

```shell
$ kubectl get apprev -l app.oam.dev/name=website
NAME           AGE
website-v1     35m
```

## Explore Components

You can explore what kinds of component definitions supported in your system.

```shell
kubectl get comp -n vela-system
NAME              WORKLOAD-KIND   DESCRIPTION                        
task              Job             Describes jobs that run code or a script to completion.                                                                                          
webservice        Deployment      Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers. 
worker            Deployment      Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.
```

The component definition objects are namespace isolated align with application, while the `vela-system` is a common system namespace of KubeVela,
definitions laid here can be used by every application. 

## Explore Traits

You can explore what kinds of trait definitions supported in your system.

```shell
$ kubectl get trait -n vela-system
NAME                                       APPLIES-TO            DESCRIPTION                                     
cpuscaler                                  [webservice worker]   configure k8s HPA with CPU metrics for Deployment
ingress                                    [webservice worker]   Configures K8s ingress and service to enable web traffic for your service. Please use route trait in cap center for advanced usage.
scaler                                     [webservice worker]   Configures replicas for your service.
sidecar                                    [webservice worker]   inject a sidecar container into your app
```

The trait definition objects are namespace isolated align with application, while the `vela-system` is a common system namespace of KubeVela,
definitions laid here can be used by every application. 