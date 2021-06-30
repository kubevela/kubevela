---
title: 检索 Applications
---

本章节我们将介绍如何检索 application 相关的资源。

## 获取 Application 列表

```shell
$ kubectl get application
NAME        COMPONENT   TYPE         PHASE     HEALTHY   STATUS   AGE
app-basic   app-basic   webservice   running   true               12d
website     frontend    webservice   running   true               4m54s
```

我们可以使用 application 缩写 `kubectl get app` 。

### 查看 Application 详情

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

以下是需要我们了解的一些重要信息：

1. `status.latestRevision` 用于显示 application 当前运行的版本。
2. `status.services` 用于显示 application 中 component 的健康状态。
3. `status.status` 用于显示 application 的全局状态。

### 获取 Application 版本

KubeVela 会对 application 对每次 spec 变更生成新版本。

```shell
$ kubectl get apprev -l app.oam.dev/name=website
NAME           AGE
website-v1     35m
```

## 检索 Components

我们可以检索出当前 KubeVela 中支持的 ComponentDefinition 列表。

```shell
kubectl get comp -n vela-system
NAME              WORKLOAD-KIND   DESCRIPTION                        
task              Job             Describes jobs that run code or a script to completion.                                                                                          
webservice        Deployment      Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers. 
worker            Deployment      Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.
```

正常情况下 ComponentDefinition 只能被同 namespace 下 application 引用，但是 `vela-system` namespace 下可以被所有 application 引用。


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

## 检索 Traits

我们可以检索出当前 KubeVela 中支持对 TraitDefinitions 。

```shell
$ kubectl get trait -n vela-system
NAME                                       APPLIES-TO            DESCRIPTION                                     
cpuscaler                                  [webservice worker]   configure k8s HPA with CPU metrics for Deployment
ingress                                    [webservice worker]   Configures K8s ingress and service to enable web traffic for your service. Please use route trait in cap center for advanced usage.
scaler                                     [webservice worker]   Configures replicas for your service.
sidecar                                    [webservice worker]   inject a sidecar container into your app
```

正常情况下 TraitDefinition 只能被同 namespace 下的 application 引用，但是 `vela-system` namespace 下的可以被所有 application 引用。

我们可以用命令 `kubectl vela show` 查看指定 TraitDefinition 暴露的参数。

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