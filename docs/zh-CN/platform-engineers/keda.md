---
title:  KEDA 作为自动伸缩 Trait
---

> 在继续之前，请确保你已了解 [Definition Objects](definition-and-templates) 和 [Defining Traits with CUE](./cue/trait) 的概念。

在下面的教程中，你将学习将 [KEDA](https://keda.sh/) 作为新的自动伸缩 trait 添加到基于 KubeVela 的平台中。

> KEDA 是基于 Kubernetes 事件驱动的自动伸缩工具。使用 KEDA，你可以根据资源指标或需要处理的事件数来驱动任何容器的伸缩。

## 步骤 1: 安装 KEDA controller

[安装 KEDA controller](https://keda.sh/docs/2.2/deploy/) 到 K8s 中。

## 步骤 2: 创建 Trait Definition

要在 KubeVela 中将 KEDA 注册为一项新功能（即 trait)，唯一需要做的就是为其创建一个 `TraitDefinition` 对象。

完整的示例可以在 [keda.yaml](https://github.com/oam-dev/catalog/blob/master/registry/keda-scaler.yaml) 中找到。
下面列出了几个要点。

### 1. 描述 Trait

```yaml
...
name: keda-scaler
annotations:
  definition.oam.dev/description: "keda supports multiple event to elastically scale applications, this scaler only applies to deployment as example"
...
```

我们使用标签 `definition.oam.dev/description` 为该 trait 添加一行描述。它将显示在帮助命令中，比如 `$ vela traits`。

### 2. 注册 API 资源

```yaml
...
spec:
  definitionRef:
    name: scaledobjects.keda.sh
...
```

这就是将 KEDA `ScaledObject` 的 API 资源声明和注册为 trait 的方式。

### 3. 定义 `appliesToWorkloads`

trait 可以附加到指定或全部的工作负载类型（`"*"` 表示你的 trait 可以与任何工作负载类型一起使用）。

对于 KEAD，我们仅允许用户将其附加到 Kubernetes 工作负载类型。 因此，我们声明如下：

```yaml
...
spec:
  ...
  appliesToWorkloads:
    - "deployments.apps" # claim KEDA based autoscaling trait can only attach to Kubernetes Deployment workload type.
...
``` 

### 4. 定义 Schematic

在这一步中，我们将定义基于 KEDA 自动伸缩 trait 的 schematic，也就是说，我们将使用简化的原语为 KEDA `ScaledObject` 创建抽象，因此平台的最终用户根本不需要知道什么是 KEDA 。


```yaml
...
schematic:
  cue:
    template: |-
      outputs: kedaScaler: {
      	apiVersion: "keda.sh/v1alpha1"
      	kind:       "ScaledObject"
      	metadata: {
      		name: context.name
      	}
      	spec: {
      		scaleTargetRef: {
      			name: context.name
      		}
      		triggers: [{
      			type: parameter.triggerType
      			metadata: {
      				type:  "Utilization"
      				value: parameter.value
      			}
      		}]
      	}
      }
      parameter: {
      	// +usage=Types of triggering application elastic scaling, Optional: cpu, memory
      	triggerType: string
      	// +usage=Value to trigger scaling actions, represented as a percentage of the requested value of the resource for the pods. like: "60"(60%)
      	value: string
      }
 ```

这是一个基于 CUE 的模板，仅开放 `type` 和 `value` 作为 trait 的属性供用户设置。

> 请查看 [Defining Trait with CUE](./cue/trait) 部分，以获取有关 CUE 模板的更多详细信息。

## 步骤 2: 向 KubeVela 注册新的 Trait  

定义文件准备就绪后，你只需将其部署到 Kubernetes 中。

```bash
$ kubectl apply -f https://raw.githubusercontent.com/oam-dev/catalog/master/registry/keda-scaler.yaml
```

用户就可以在 `Application` 中立即使用新 trait。

