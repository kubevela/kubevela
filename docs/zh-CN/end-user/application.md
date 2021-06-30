---
title: Application
---

本文档将介绍如何使用 KubeVela 设计一个没有定义任何策略或放置规则的简单 Application。

> 注意：由于您没有声明放置规则，KubeVela 会将此应用程序直接部署到控制平面集群（即您的 `kubectl` 正在与之通信的集群）。 如果你使用 KinD 或 MiniKube 等本地集群来玩 KubeVela，也是同样的情况。

## 步骤 1：检查可用组件

组件是组成应用程序的可部署或可配置实体。 它可以是 Helm chart、简单的 Kubernetes 工作负载、CUE 或 Terraform 模块或云数据库等。

让我们检查一下全新 KubeVela 中的可用组件。

```shell
kubectl get comp -n vela-system
```
```console
NAME              WORKLOAD-KIND   DESCRIPTION                        
task              Job             Describes jobs that run code or a script to completion.                                                                                          
webservice        Deployment      Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers. 
worker            Deployment      Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.
```

要显示给定组件的规范，您可以使用 `vela show`。

```shell
kubectl vela show webservice
```
```console
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
... // skip other fields
```

> 提示：`vela show xxx --web` 将在您的默认浏览器中打开其功能参考文档。

您可以随时向平台[添加更多组件](components/more)。

## 步骤 2：声明一个 Application

Application 是部署的完整描述。 让我们定义一个部署 *Web Service* 和 *Worker* 组件的 Application。

```yaml
# sample.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend
      type: webservice
      properties:
        image: nginx
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
```

## 步骤 3：附加 Trait

Trait 是平台提供的功能，可以*Overlay*具有额外操作行为的给定组件。

```shell
kubectl get trait -n vela-system
```
```console
NAME                                       APPLIES-TO            DESCRIPTION                                     
cpuscaler                                  [webservice worker]   Automatically scale the component based on CPU usage.
ingress                                    [webservice worker]   Enable public web traffic for the component.
scaler                                     [webservice worker]   Manually scale the component.
sidecar                                    [webservice worker]   Inject a sidecar container to the component.
```

让我们检查一下 `sidecar` trait 的规范。

```shell
kubectl vela show sidecar
```
```console
# Properties
+---------+-----------------------------------------+----------+----------+---------+
|  NAME   |               DESCRIPTION               |   TYPE   | REQUIRED | DEFAULT |
+---------+-----------------------------------------+----------+----------+---------+
| name    | Specify the name of sidecar container   | string   | true     |         |
| image   | Specify the image of sidecar container  | string   | true     |         |
| command | Specify the commands run in the sidecar | []string | false    |         |
+---------+-----------------------------------------+----------+----------+---------+
```

请注意，Trait 被设计为可以 *Overlay*。

这意味着对于 `sidecar` trait，你的 `frontend` 组件不需要有一个 sidecar 模板或带一个 webhook 来启用 sidecar 注入。 相反，KubeVela 能够在组件生成 sidecar 之后（无论是 Helm chart 还是 CUE 模块），但在将 sidecar 应用于运行时集群之前，将其修补到其工作负载实例。

同样，系统将根据您设置的属性分配一个 HPA 实例并将其“链接”到目标工作负载实例，组件本身不受影响。

现在让我们将 `sidecar` 和 `cpuscaler` trait 附加到 `frontend` 组件。


```yaml
# sample.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: website
spec:
  components:
    - name: frontend              # This is the component I want to deploy
      type: webservice
      properties:
        image: nginx
      traits:
        - type: cpuscaler         # Automatically scale the component by CPU usage after deployed
          properties:
            min: 1
            max: 10
            cpuPercent: 60
        - type: sidecar           # Inject a fluentd sidecar before applying the component to runtime cluster
          properties:
            name: "sidecar-test"
            image: "fluentd"
    - name: backend
      type: worker
      properties:
        image: busybox
        cmd:
          - sleep
          - '1000'
```

## 步骤 4：部署 Application

```shell
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/enduser/sample.yaml
```
```console
application.core.oam.dev/website created
```

你会得到 Application 变成`running`。

```shell
kubectl get application
```
```console
NAME        COMPONENT   TYPE         PHASE     HEALTHY   STATUS   AGE
website     frontend    webservice   running   true               4m54s
```

检查 Application 的详细信息。

```shell
kubectl get app website -o yaml
```
```console
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

具体来说：

1. `status.latestRevision` 声明此部署的当前版本。
2. `status.services` 声明本次部署创建的组件和健康状态。
3. `status.status` 声明了这个部署的全局状态。

### 列出修订

更新应用程序实体时，KubeVela 将为此更改创建一个新修订。

```shell
kubectl get apprev -l app.oam.dev/name=website
```
```console
NAME           AGE
website-v1     35m
```

此外，系统将根据附加的 [rollout plan](scopes/rollout-plan) 决定如何/是否部署应用程序。
