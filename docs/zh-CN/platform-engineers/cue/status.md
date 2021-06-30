---
title:  状态回写
---

本文档将说明如何通过在定义对象中使用 CUE 模板来实现状态回写。 

## 健康检查

在 Workload 和 Trait 中健康检查字段都是 `spec.status.healthPolicy`。

如果没有定义该字段，健康检查结果默认为 `true`。

CUE 中关键字为 `isHealth`，CUE 表达式结果必须是 `bool` 类型。
KubeVela 运行时将定期评估 CUE 表达式直到状态为健康。控制器每次都会获取所有 Kubernetes 资源，同时将结果填充到 context 字段中。

所以 context 字段将包含以下信息：

```cue
context:{
  name: <component name>
  appName: <app name>
  output: <K8s workload resource>
  outputs: {
    <resource1>: <K8s trait resource1>
    <resource2>: <K8s trait resource2>
  }
}
```

Trait 并不包含 `context.ouput` 字段，其他字段都是相同。

以下为健康检查的示例：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
spec:
  status:
    healthPolicy: |
      isHealth: (context.output.status.readyReplicas > 0) && (context.output.status.readyReplicas == context.output.status.replicas)
   ...
```

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
spec:
  status:
    healthPolicy: |
      isHealth: len(context.outputs.service.spec.clusterIP) > 0
   ...
```

> Component 健康检查示例请参考 [这篇文章](https://github.com/oam-dev/kubevela/blob/master/docs/examples/app-with-status/template.yaml) 。

该健康检查结果将被记录在组件对应的 `Application` 资源中。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  components:
  - name: myweb
    type: worker    
    properties:
      cmd:
      - sleep
      - "1000"
      enemies: alien
      image: busybox
      lives: "3"
    traits:
    - type: ingress
      properties:
        domain: www.example.com
        http:
          /: 80
status:
  ...
  services:
  - healthy: true
    message: "type: busybox,\t enemies:alien"
    name: myweb
    traits:
    - healthy: true
      message: 'Visiting URL: www.example.com, IP: 47.111.233.220'
      type: ingress
  status: running
```

## 自定义状态

自定义状态配置项为 `spec.status.customStatus`，Workload 和 Trait 中都是该字段。

自定义状态 CUE 中关键词为 `message`，CUE 表达式的结果必须是 `string` 类型。

自定义状态的内部机制类似上面介绍的健康检查。Application CRD 控制器将评估 CUE 表达式直到检查成功。

context 字段将包含以下信息：

```cue
context:{
  name: <component name>
  appName: <app name>
  output: <K8s workload resource>
  outputs: {
    <resource1>: <K8s trait resource1>
    <resource2>: <K8s trait resource2>
  }
}
```

Trait 并不包含 `context.ouput` 字段，其他字段都是相同。

Component 健康检查示例请参考 [这篇文章](https://github.com/oam-dev/kubevela/blob/master/docs/examples/app-with-status/template.yaml) 。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
spec:
  status:
    customStatus: |-
      message: "type: " + context.output.spec.template.spec.containers[0].image + ",\t enemies:" + context.outputs.gameconfig.data.enemies
   ...
```

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
spec:
  status:
    customStatus: |-
      message: "type: "+ context.outputs.service.spec.type +",\t clusterIP:"+ context.outputs.service.spec.clusterIP+",\t ports:"+ "\(context.outputs.service.spec.ports[0].port)"+",\t domain"+context.outputs.ingress.spec.rules[0].host
   ...
```
