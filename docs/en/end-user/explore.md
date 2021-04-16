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

You can use [vela kubectl plugin](./kubectlplugin) to view the detail usage of specific component definition.

```shell
$ kubectl vela show webservice
# Properties
+------------------+----------------------------------------------------------------------------------+-----------------------+----------+---------+
|       NAME       |                                   DESCRIPTION                                    |         TYPE          | REQUIRED | DEFAULT |
+------------------+----------------------------------------------------------------------------------+-----------------------+----------+---------+
| cmd              | Commands to run in the container                                                 | []string              | false    |         |
| env              | Define arguments by using environment variables                                  | [[]env](#env)         | false    |         |
| addRevisionLabel |                                                                                  | bool                  | true     | false   |
| image            | Which image would you like to use for your service                               | string                | true     |         |
| port             | Which port do you want customer traffic sent to                                  | int                   | true     |      80 |
| cpu              | Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core) | string                | false    |         |
| volumes          | Declare volumes and volumeMounts                                                 | [[]volumes](#volumes) | false    |         |
+------------------+----------------------------------------------------------------------------------+-----------------------+----------+---------+


##### volumes
+-----------+---------------------------------------------------------------------+--------+----------+---------+
|   NAME    |                             DESCRIPTION                             |  TYPE  | REQUIRED | DEFAULT |
+-----------+---------------------------------------------------------------------+--------+----------+---------+
| name      |                                                                     | string | true     |         |
| mountPath |                                                                     | string | true     |         |
| type      | Specify volume type, options: "pvc","configMap","secret","emptyDir" | string | true     |         |
+-----------+---------------------------------------------------------------------+--------+----------+---------+


## env
+-----------+-----------------------------------------------------------+-------------------------+----------+---------+
|   NAME    |                        DESCRIPTION                        |          TYPE           | REQUIRED | DEFAULT |
+-----------+-----------------------------------------------------------+-------------------------+----------+---------+
| name      | Environment variable name                                 | string                  | true     |         |
| value     | The value of the environment variable                     | string                  | false    |         |
| valueFrom | Specifies a source the value of this var should come from | [valueFrom](#valueFrom) | false    |         |
+-----------+-----------------------------------------------------------+-------------------------+----------+---------+


### valueFrom
+--------------+--------------------------------------------------+-------------------------------+----------+---------+
|     NAME     |                   DESCRIPTION                    |             TYPE              | REQUIRED | DEFAULT |
+--------------+--------------------------------------------------+-------------------------------+----------+---------+
| secretKeyRef | Selects a key of a secret in the pod's namespace | [secretKeyRef](#secretKeyRef) | true     |         |
+--------------+--------------------------------------------------+-------------------------------+----------+---------+


#### secretKeyRef
+------+------------------------------------------------------------------+--------+----------+---------+
| NAME |                           DESCRIPTION                            |  TYPE  | REQUIRED | DEFAULT |
+------+------------------------------------------------------------------+--------+----------+---------+
| name | The name of the secret in the pod's namespace to select from     | string | true     |         |
| key  | The key of the secret to select from. Must be a valid secret key | string | true     |         |
+------+------------------------------------------------------------------+--------+----------+---------+
```

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

You can use `kubectl vela show` to see the usage of specific trait definition.

```shell
$ kubectl vela show sidecar
# Properties
+---------+-----------------------------------------+----------+----------+---------+
|  NAME   |               DESCRIPTION               |   TYPE   | REQUIRED | DEFAULT |
+---------+-----------------------------------------+----------+----------+---------+
| name    | Specify the name of sidecar container   | string   | true     |         |
| image   | Specify the image of sidecar container  | string   | true     |         |
| command | Specify the commands run in the sidecar | []string | false    |         |
+---------+-----------------------------------------+----------+----------+---------+
```