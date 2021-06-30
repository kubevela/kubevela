---
title:  定义CRD
---

在本部分中，我们会对 `ComponentDefinition` 和 `TraitDefinition` 进行详细介绍。

> 所有的定义对象都应由平台团队来进行维护和安装。在此背景下，可以把平台团队理解为平台中的*能力提供者*。

## 概述

本质上，KubeVela 中的定义对象由三个部分组成：

- **能力指示器 （Capability Indicator）**
  - `ComponentDefinition` 使用 `spec.workload` 指出此组件的 workload 类型.
  - `TraitDefinition` 使用 `spec.definitionRef` 指出此 trait 的提供者。
- **互操作字段 （Interoperability Fields）**
  - 他们是为平台所设计的，用来确保给定的 workload 类型可以和某个 trait 一起工作。因此只有 `TraitDefinition` 有这些字段。
- **能力封装和抽象 （Capability Encapsulation and Abstraction）** （由 `spec.schematic` 定义）
  - 它定义了此 capability 的**模板和参数** ，比如封装。

因此，定义对象的基本结构如下所示：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: XxxDefinition
metadata:
  name: <definition name>
spec:
  ...
  schematic:
    cue:
      # cue template ...
    helm:
      # Helm chart ...
  # ... interoperability fields
```

我们接下来详细解释每个字段。

### 能力指示器 （Capability Indicator）

在 `ComponentDefinition` 中，workload 类型的指示器被声明为 `spec.workload`

下面的示例是在 KubeVela 中，一个给 *Web Service* 的定义：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webservice
  namespace: default
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
    ...        
```

在上面的示例中，它声称利用 Kubernetes 的 Deployment （`apiVersion: apps/v1`, `kind: Deployment`）作为组件的 workload 类型。

### 互操作字段 （Interoperability Fields）

**只有 trait** 有互操作字段。在一个 `TraitDefinition` 中，互操作字段的大体示例如下所示：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name:  ingress
spec:
  appliesToWorkloads: 
    - deployments.apps
    - webservice
  conflictsWith: 
    - service
  workloadRefPath: spec.wrokloadRef
  podDisruptive: false
```

我们来详细解释一下。

#### `.spec.appliesToWorkloads`

该字段定义了此 trait 允许应用于哪些类型的 workload 的约束。

- 它使用一个字符串的数组作为其值。
- 数组中的每一个元素指向允许应用此 trait 的一个或一组 workload 类型。

有四种来表示一个或者一组 workload 类型。

- `ComponentDefinition` 命名， 例如 `webservice`， `worker`
- `ComponentDefinition` 定义引用（CRD 命名），例如 `deployments.apps`
- 以`*.`为前缀的 `ComponentDefinition` 定义引用的资源组，例如`*.apps` 和 `*.oam.dev`。这表示 trait 被允许应用于该组中的任意 workload。
- `*` 表示 trait 被允许应用于任意 workload。

如果省略此字段，则表示该 trait 允许应用于任意 workload 类型。

如果将一个 trait 应用于未包含在 `appliesToWorkloads` 中的 workload，KubeVela 将会报错。

##### `.spec.conflictsWith`

如果将某些种类的 trait 应用于该 workload，该字段定义了其中哪些 trait 与该 trait 冲突的约束。

- 它使用一个字符串的数组作为其值。
- 数组中的每一个元素指向一个或一组 trait。

有四种来表示一个或者一组 workload 类型。

- `TraitDefinition` 命名，比如 `ingress`
- 以`*.`为前缀的 `TraitDefinition` 定义引用的资源组，例如`*.networking.k8s.io`。这表示当前 trait 与该组中的任意 trait 相冲突。
- `*` 表示当前 trait 与任意 trait 相冲突。

如果省略此字段，则表示该 trait 没有和其他任何 trait 相冲突。

##### `.spec.workloadRefPath`

该字段定义 trait 的字段路径，该路径用于存储对其应用 trait 的 workload 的引用。

- 它使用一个字符串作为其值，比如 `spec.workloadRef`.

如果设置了此字段，KubeVela core 会自动将 workload 引用填充到 trait 的目标字段中。然后，trait controller 可以之后从 trait 中获取 workload 引用。因此，此字段通常和 trait 一起出现，其 controller 在运行时依赖于 workload 引用。

如何设置此字段的具体细节，请查阅 [scaler](https://github.com/oam-dev/kubevela/blob/master/charts/vela-core/templates/defwithtemplate/manualscale.yaml) trait 作为演示。

##### `.spec.podDisruptive`

此字段定义了添加或者更新 trait 会不会破坏 pod。在此示例中，因为答案是不会，所以该字段为 `false`，当添加或更新 trait 时，它不会影响 pod。如果此字段是 `true`，则它会导致 pod 在 trait 被添加或者更新时被破坏并重启。默认情况下，该值为 `false`，这意味着该 trait 不会影响 pod。请小心处理此字段。对于严肃的大规模生产使用场景而言，它非常重要和有用。

### 能力封装和抽象 （Capability Encapsulation and Abstraction）

给定的能力的模板和封装被定义在 `spec.schematic` 字段中。如下所示范例是一个 KubeVela 中的*Web Service* 类型的完整定义：

<details>

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webservice
  namespace: default
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
        output: {
            apiVersion: "apps/v1"
            kind:       "Deployment"
            spec: {
                selector: matchLabels: {
                    "app.oam.dev/component": context.name
                }
        
                template: {
                    metadata: labels: {
                        "app.oam.dev/component": context.name
                    }
        
                    spec: {
                        containers: [{
                            name:  context.name
                            image: parameter.image
        
                            if parameter["cmd"] != _|_ {
                                command: parameter.cmd
                            }
        
                            if parameter["env"] != _|_ {
                                env: parameter.env
                            }
        
                            if context["config"] != _|_ {
                                env: context.config
                            }
        
                            ports: [{
                                containerPort: parameter.port
                            }]
        
                            if parameter["cpu"] != _|_ {
                                resources: {
                                    limits:
                                        cpu: parameter.cpu
                                    requests:
                                        cpu: parameter.cpu
                                }
                            }
                        }]
                }
                }
            }
        }
        parameter: {
            // +usage=Which image would you like to use for your service
            // +short=i
            image: string
        
            // +usage=Commands to run in the container
            cmd?: [...string]
        
            // +usage=Which port do you want customer traffic sent to
            // +short=p
            port: *80 | int
            // +usage=Define arguments by using environment variables
            env?: [...{
                // +usage=Environment variable name
                name: string
                // +usage=The value of the environment variable
                value?: string
                // +usage=Specifies a source the value of this var should come from
                valueFrom?: {
                    // +usage=Selects a key of a secret in the pod's namespace
                    secretKeyRef: {
                        // +usage=The name of the secret in the pod's namespace to select from
                        name: string
                        // +usage=The key of the secret to select from. Must be a valid secret key
                        key: string
                    }
                }
            }]
            // +usage=Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core)
            cpu?: string
        }     
```

</details>

`schematic` 的技术规范在接下来的 CUE 和 Helm 相关的文档中有详细解释。

同时，`schematic` 字段使你可以直接根据他们来渲染UI表单。详细操作请见[从定义中生成表单](/docs/platform-engineers/openapi-v3-json-schema)部分。
