---
title:  怎么用
---

在本节中，将介绍如何使用 raw K8s Object 通过 `ComponentDefinition` 来声明应用程序组件。

> 在阅读本部分之前，请确保您已经了解了[定义和模板概念](../definition-and-templates)。

## 声明`ComponentDefinition`

这是一个基于“ComponentDefinition”的原始模板示例，它提供了对工作负载类型的抽象：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: kube-worker
  namespace: default
spec:
  workload: 
    definition: 
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    kube: 
      template:
        apiVersion: apps/v1
        kind: Deployment
        spec:
          selector:
            matchLabels:
              app: nginx
          template:
            metadata:
              labels:
                app: nginx
            spec:
              containers:
              - name: nginx
                ports:
                - containerPort: 80 
      parameters: 
      - name: image
        required: true
        type: string
        fieldPaths: 
        - "spec.template.spec.containers[0].image"
```

详细地说，`.spec.schematic.kube` 包含工作负载资源的模板和
可配置的参数。
- `.spec.schematic.kube.template` 是 YAML 格式的原始模板。
- `.spec.schematic.kube.parameters` 包含一组可配置的参数。 `name`、`type` 和 `fieldPaths` 是必填字段，`description` 和 `required` 是可选字段。
  - 参数`name` 在`ComponentDefinition` 中必须是唯一的。
  - `type` 表示设置到字段的值的数据类型。这是一个必填字段，它将帮助 KubeVela 自动为参数生成 OpenAPI JSON 模式。在原始模板中，只允许使用基本数据类型，包括 `string`、`number` 和 `boolean`，而不允许使用 `array` 和 `object`。
  - 参数中的`fieldPaths` 指定模板中的字段数组，这些字段将被该参​​数的值覆盖。字段被指定为没有前导点的 JSON 字段路径，例如
`spec.replicas`、`spec.containers[0].image`。

## 声明一个 `Application`

这是一个示例 `Application`。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: mycomp
      type: kube-worker
      properties: 
        image: nginx:1.14.0
```

由于参数只支持基本数据类型，`properties` 中的值应该是简单的键值，`<parameterName>: <parameterValue>`。

部署“应用程序”并验证正在运行的工作负载实例。

```shell
$ kubectl get deploy
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
mycomp                   1/1     1            1           66m
```
并检查参数是否有效。
```shell
$ kubectl get deployment mycomp -o json | jq '.spec.template.spec.containers[0].image'
"nginx:1.14.0"
```

